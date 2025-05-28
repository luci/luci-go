// Copyright 2021 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package changelist

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
)

// BatchOnCLUpdatedTaskClass is the Task Class ID of the BatchOnCLUpdatedTask,
// which is enqueued during CL mutations.
const BatchOnCLUpdatedTaskClass = "batch-notify-on-cl-updated"

// Mutator modifies CLs and guarantees at least once notification of relevant CV
// components.
//
// All CL entities in production code must be modified via the Mutator.
//
// Mutator notifies 2 CV components: Run and Project managers.
// In the future, it'll also notify Tryjob Manager.
//
// Run Manager is notified for each IncompleteRuns in the **new** CL version.
//
// Project manager is notified in following cases:
//  1. On the project in the context of which the CL is being modified.
//  2. On the project which owns the Snapshot of the *prior* CL version (if it
//     had any Snapshot).
//
// When the number of notifications is large, Mutator may chose to
// transactionally enqueue a TQ task, which will send notifications in turn.
type Mutator struct {
	tqd *tq.Dispatcher
	pm  pmNotifier
	rm  rmNotifier
	tj  tjNotifier
}

func NewMutator(tqd *tq.Dispatcher, pm pmNotifier, rm rmNotifier, tj tjNotifier) *Mutator {
	m := &Mutator{tqd, pm, rm, tj}
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           BatchOnCLUpdatedTaskClass,
		Queue:        "notify-on-cl-updated",
		Prototype:    &BatchOnCLUpdatedTask{},
		Kind:         tq.Transactional,
		Quiet:        true,
		QuietOnError: true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*BatchOnCLUpdatedTask)
			err := m.handleBatchOnCLUpdatedTask(ctx, task)
			return common.TQifyError(ctx, err)
		},
	})
	return m
}

// pmNotifier encapsulates interaction with Project Manager.
//
// In production, implemented by prjmanager.Notifier.
type pmNotifier interface {
	NotifyCLsUpdated(ctx context.Context, project string, events *CLUpdatedEvents) error
}

// rmNotifier encapsulates interaction with Run Manager.
//
// In production, implemented by run.Notifier.
type rmNotifier interface {
	NotifyCLsUpdated(ctx context.Context, rid common.RunID, events *CLUpdatedEvents) error
}

type tjNotifier interface {
	ScheduleCancelStale(ctx context.Context, clid common.CLID, gtePatchset, ltPatchset int32, eta time.Time) error
}

// ErrStopMutation is a special error used by MutateCallback to signal that no
// mutation is necessary.
//
// This is very useful because the datastore.RunInTransaction(ctx, f, ...)
// does retries by default which combined with submarine writes (transaction
// actually succeeded, but the client didn't get to know, e.g. due to network
// flake) means an idempotent MutateCallback can avoid noop updates yet still
// keep the code clean and readable. For example,
//
// ```
//
//	cl, err := mu.Update(ctx, project, clid, func (cl *changelist.CL) error {
//	  if cl.Snapshot == nil {
//	    return ErrStopMutation // noop
//	  }
//	  cl.Snapshot = nil
//	  return nil
//	})
//
//	if err != nil {
//	  return errors.Annotate(err, "failed to reset Snapshot").Err()
//	}
//
// doSomething(ctx, cl)
// ```
var ErrStopMutation = errors.New("stop CL mutation")

// MutateCallback is called by Mutator to mutate the CL inside a transaction.
//
// The function should be idempotent.
//
// If no error is returned, Mutator proceeds saving the CL.
//
// If special ErrStopMutation is returned, Mutator aborts the transaction and
// returns existing CL read from Datastore and no error. In the special case of
// Upsert(), the returned CL may actually be nil if CL didn't exist.
//
// If any error is returned other than ErrStopMutation, Mutator aborts the
// transaction and returns nil CL and the exact same error.
type MutateCallback func(cl *CL) error

// Upsert creates new or updates existing CL via a dedicated transaction in the
// context of the given LUCI project.
//
// Prefer to use Update if CL ID is known.
//
// If CL didn't exist before, the callback is provided a CL with temporarily
// reserved ID. Until Upsert returns with success, this ID is not final,
// but it's fine to use it in other entities saved within the same transaction.
//
// If CL didn't exist before and the callback returns ErrStopMutation, then
// Upsert returns (nil, nil).
func (m *Mutator) Upsert(ctx context.Context, project string, eid ExternalID, clbk MutateCallback) (*CL, error) {
	// Quick path in case CL already exists, which is a common case,
	// and can usually be satisfied by dscache lookup.
	mapEntity := clMap{ExternalID: eid}
	switch err := datastore.Get(ctx, &mapEntity); {
	case err == datastore.ErrNoSuchEntity:
		// OK, proceed to slow path below.
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to get clMap entity %q: %w", eid, err))
	default:
		return m.Update(ctx, project, mapEntity.InternalID, clbk)
	}

	var result *CL
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		// Check if CL exists and prepare appropriate clMutation.
		var clMutation *CLMutation
		mapEntity := clMap{ExternalID: eid}
		switch err := datastore.Get(ctx, &mapEntity); {
		case err == datastore.ErrNoSuchEntity:
			clMutation, err = m.beginInsert(ctx, project, eid)
			if err != nil {
				return err
			}
		case err != nil:
			return transient.Tag.Apply(errors.Fmt("failed to get clMap entity %q: %w", eid, err))
		default:
			clMutation, err = m.Begin(ctx, project, mapEntity.InternalID)
			if err != nil {
				return err
			}
			result = clMutation.CL
		}
		if err := clbk(clMutation.CL); err != nil {
			return err
		}
		result, err = clMutation.Finalize(ctx)
		return err
	}, nil)
	switch {
	case innerErr == ErrStopMutation:
		return result, nil
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to commit Upsert of CL %q: %w", eid, err))
	default:
		return result, nil
	}
}

// Update mutates one CL via a dedicated transaction in the context of the given
// LUCI project.
//
// If the callback returns ErrStopMutation, then Update returns the read CL
// entity and nil error.
func (m *Mutator) Update(ctx context.Context, project string, id common.CLID, clbk MutateCallback) (*CL, error) {
	var result *CL
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		clMutation, err := m.Begin(ctx, project, id)
		if err != nil {
			return err
		}
		result = clMutation.CL
		if err := clbk(clMutation.CL); err != nil {
			return err
		}
		result, err = clMutation.Finalize(ctx)
		return err
	}, nil)
	switch {
	case innerErr == ErrStopMutation:
		return result, nil
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to commit update on CL %d: %w", id, err))
	default:
		return result, nil
	}
}

// CLMutation encapsulates one CL mutation.
type CLMutation struct {
	// CL can be modified except the following fields:
	//  * ID
	//  * ExternalID
	//  * EVersion
	//  * UpdateTime
	CL *CL

	// m is a back reference to its parent -- Mutator.
	m *Mutator

	// trans is only to detect incorrect usage.
	trans datastore.Transaction
	// project in the context of which CL is modified.
	project string

	id         common.CLID
	externalID ExternalID

	priorEversion              int64
	priorUpdateTime            time.Time
	priorProject               string
	priorMinEquivalentPatchset int32
}

func (m *Mutator) beginInsert(ctx context.Context, project string, eid ExternalID) (*CLMutation, error) {
	clMutation := &CLMutation{
		CL:      &CL{ExternalID: eid},
		m:       m,
		trans:   datastore.CurrentTransaction(ctx),
		project: project,
	}
	if err := datastore.AllocateIDs(ctx, clMutation.CL); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("failed to allocate new CL ID for %q: %w", eid, err))
	}
	if err := datastore.Put(ctx, &clMap{ExternalID: eid, InternalID: clMutation.CL.ID}); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("failed to insert clMap entity for %q: %w", eid, err))
	}
	clMutation.backup()
	return clMutation, nil
}

// Begin starts mutation of one CL inside an existing transaction in the context of
// the given LUCI project.
func (m *Mutator) Begin(ctx context.Context, project string, id common.CLID) (*CLMutation, error) {
	clMutation := &CLMutation{
		CL:      &CL{ID: id},
		m:       m,
		trans:   datastore.CurrentTransaction(ctx),
		project: project,
	}
	if clMutation.trans == nil {
		return nil, errors.New("changelist.Mutator.Begin must be called inside an existing Datastore transaction")
	}
	switch err := datastore.Get(ctx, clMutation.CL); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.Fmt("CL %d doesn't exist: %w", id, err)
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to get CL %d: %w", id, err))
	}
	clMutation.backup()
	return clMutation, nil
}

// Adopt starts a mutation of a given CL which was just read from Datastore.
//
// CL must have been loaded in the same Datastore transaction.
// CL must have been kept read-only after loading. It's OK to modify it after
// CLMutation is returned.
//
// Adopt exists when there is substantial advantage in batching loading of CL
// and non-CL entities in a single Datastore RPC.
// Prefer to use Begin unless performance consideration is critical.
func (m *Mutator) Adopt(ctx context.Context, project string, cl *CL) *CLMutation {
	clMutation := &CLMutation{
		CL:      cl,
		m:       m,
		trans:   datastore.CurrentTransaction(ctx),
		project: project,
	}
	if clMutation.trans == nil {
		panic(fmt.Errorf("changelist.Mutator.Adopt must be called inside an existing Datastore transaction"))
	}
	clMutation.backup()
	return clMutation
}

func (clm *CLMutation) backup() {
	clm.id = clm.CL.ID
	clm.externalID = clm.CL.ExternalID
	clm.priorEversion = clm.CL.EVersion
	clm.priorUpdateTime = clm.CL.UpdateTime
	if p := clm.CL.Snapshot.GetLuciProject(); p != "" {
		clm.priorProject = p
	}
	clm.priorMinEquivalentPatchset = clm.CL.Snapshot.GetMinEquivalentPatchset()
}

// Finalize finalizes CL mutation.
//
// Must be called at most once.
// Must be called in the same Datastore transaction as Begin() which began the
// CL mutation.
func (clm *CLMutation) Finalize(ctx context.Context) (*CL, error) {
	if err := clm.finalize(ctx); err != nil {
		return nil, err
	}
	if err := datastore.Put(ctx, clm.CL); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("failed to put CL %d: %w", clm.id, err))
	}
	if err := clm.m.dispatchBatchNotify(ctx, clm); err != nil {
		return nil, err
	}
	return clm.CL, nil
}

func (clm *CLMutation) finalize(ctx context.Context) error {
	switch t := datastore.CurrentTransaction(ctx); {
	case clm.trans == nil:
		return errors.New("changelist.CLMutation.Finalize called the second time")
	case t == nil:
		return errors.New("changelist.CLMutation.Finalize must be called inside an existing Datastore transaction")
	case t != clm.trans:
		return errors.New("changelist.CLMutation.Finalize called inside a different Datastore transaction")
	}
	clm.trans = nil

	switch {
	case clm.id != clm.CL.ID:
		return errors.New("CL.ID must not be modified")
	case clm.externalID != clm.CL.ExternalID:
		return errors.New("CL.ExternalID must not be modified")
	case clm.priorEversion != clm.CL.EVersion:
		return errors.New("CL.EVersion must not be modified")
	case !clm.priorUpdateTime.Equal(clm.CL.UpdateTime):
		return errors.New("CL.UpdateTime must not be modified")
	}
	clm.CL.EVersion++
	clm.CL.UpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
	clm.CL.UpdateRetentionKey()
	return nil
}

// BeginBatch starts a batch of CL mutations within the same Datastore
// transaction.
func (m *Mutator) BeginBatch(ctx context.Context, project string, ids common.CLIDs) ([]*CLMutation, error) {
	trans := datastore.CurrentTransaction(ctx)
	if trans == nil {
		return nil, errors.New("changelist.Mutator.BeginBatch must be called inside an existing Datastore transaction")
	}
	cls, err := LoadCLsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	muts := make([]*CLMutation, len(ids))
	for i, cl := range cls {
		muts[i] = &CLMutation{
			CL:      cl,
			m:       m,
			trans:   trans,
			project: project,
		}
		muts[i].backup()
	}
	return muts, nil
}

// FinalizeBatch finishes a batch of CL mutations within the same Datastore
// transaction.
//
// The given mutations can originate from either Begin or BeginBatch calls.
// The only requirement is that they must all originate within the current
// Datastore transaction.
//
// It also transactionally schedules tasks to cancel stale tryjobs for any
// CLs in the batch whose minEquivPatchset has changed.
func (m *Mutator) FinalizeBatch(ctx context.Context, muts []*CLMutation) ([]*CL, error) {
	cls := make([]*CL, len(muts))
	for i, mut := range muts {
		if err := mut.finalize(ctx); err != nil {
			return nil, err
		}
		cls[i] = mut.CL
	}
	if err := datastore.Put(ctx, cls); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("failed to put %d CLs: %w", len(cls), err))
	}
	if err := m.dispatchBatchNotify(ctx, muts...); err != nil {
		return nil, err
	}
	return cls, nil
}

///////////////////////////////////////////////////////////////////////////////
// Internal implementation of notification dispatch.

// projects returns which LUCI projects to notify.
func (clm *CLMutation) projects() []string {
	if clm.priorProject != "" && clm.project != clm.priorProject {
		return []string{clm.project, clm.priorProject}
	}
	return []string{clm.project}
}

func (m *Mutator) dispatchBatchNotify(ctx context.Context, muts ...*CLMutation) error {
	batch := &BatchOnCLUpdatedTask{
		// There are usually at most 2 Projects and 2 Runs being notified.
		Projects: make(map[string]*CLUpdatedEvents, 2),
		Runs:     make(map[string]*CLUpdatedEvents, 2),
	}
	for _, mut := range muts {
		e := &CLUpdatedEvent{Clid: int64(mut.CL.ID), Eversion: mut.CL.EVersion}
		for _, p := range mut.projects() {
			batch.Projects[p] = batch.Projects[p].append(e)
		}
		for _, r := range mut.CL.IncompleteRuns {
			batch.Runs[string(r)] = batch.Runs[string(r)].append(e)
		}

		switch {
		case mut.CL.Snapshot == nil: // do nothing if snapshot is not avaiable
		case mut.priorMinEquivalentPatchset != 0 && mut.priorMinEquivalentPatchset < mut.CL.Snapshot.GetMinEquivalentPatchset():
			// add 1 second delay to allow run to finalize so that Tryjobs can be
			// cancelled right away.
			eta := clock.Now(ctx).UTC().Add(1 * time.Second)
			if err := m.tj.ScheduleCancelStale(ctx, mut.id, mut.priorMinEquivalentPatchset, mut.CL.Snapshot.GetMinEquivalentPatchset(), eta); err != nil {
				return err
			}
		case mut.CL.Snapshot.GetGerrit().GetInfo().GetStatus() == gerritpb.ChangeStatus_ABANDONED:
			// Cancelling Tryjobs if CL is abandoned. The basic idea is that when
			// CL is abandoned, LUCI CV needs to cancel any Tryjob that runs between
			// the current minimum equivalent patchset and the curent latest patchset.
			// It assumes tryjobs for all patchsets before the current minimum
			// equivalent patchset have been cancelled by the swtich case above.

			// add 1 second delay to allow run to finalize so that Tryjobs can be
			// cancelled right away.
			eta := clock.Now(ctx).UTC().Add(1 * time.Second)
			if err := m.tj.ScheduleCancelStale(ctx, mut.id,
				mut.CL.Snapshot.GetMinEquivalentPatchset(),
				mut.CL.Snapshot.GetPatchset()+1,
				eta); err != nil {
				return err
			}
		}
	}
	err := m.tqd.AddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%s/%d-cls/%d-prjs/%d-runs", muts[0].project, len(muts), len(batch.GetProjects()), len(batch.GetRuns())),
		Payload: batch,
	})
	if err != nil {
		return errors.Fmt("failed to add BatchOnCLUpdatedTask to TQ: %w", err)
	}
	return nil
}

func (m *Mutator) handleBatchOnCLUpdatedTask(ctx context.Context, batch *BatchOnCLUpdatedTask) error {
	errs := parallel.WorkPool(min(16, len(batch.GetProjects())+len(batch.GetRuns())), func(work chan<- func() error) {
		for project, events := range batch.GetProjects() {
			work <- func() error { return m.pm.NotifyCLsUpdated(ctx, project, events) }
		}
		for run, events := range batch.GetRuns() {
			work <- func() error { return m.rm.NotifyCLsUpdated(ctx, common.RunID(run), events) }
		}
	})
	return common.MostSevereError(errs)
}

func (b *CLUpdatedEvents) append(e *CLUpdatedEvent) *CLUpdatedEvents {
	if b == nil {
		return &CLUpdatedEvents{Events: []*CLUpdatedEvent{e}}
	}
	b.Events = append(b.Events, e)
	return b
}
