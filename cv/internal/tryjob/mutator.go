// Copyright 2024 The LUCI Authors.
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

package tryjob

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

// Mutator modifies Tryjobs and guarantees at least once notification of
// relevant CV components.
//
// All Tryjob entities must be modified via the Mutator unless it's in the
// test. It's NOT necessary to create a Tryjob using the mutator unless the
// Tryjob needs to be created with an external ID associated. In that case,
// please use `Upsert` method as it will ensure only 1 Tryjob entity ever
// be created for a given external ID.
//
// Mutator currently only notifies the RunManager for all Runs concerning
// the modified Tryjob(s)
type Mutator struct {
	rm rmNotifier
}

// NewMutator creates a new Mutator instance.
func NewMutator(rm rmNotifier) *Mutator {
	m := &Mutator{rm}
	return m
}

// rmNotifier encapsulates interaction with Run Manager.
//
// In production, implemented by run.Notifier.
type rmNotifier interface {
	NotifyTryjobsUpdated(ctx context.Context, runID common.RunID, tryjobs *TryjobUpdatedEvents) error
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
//	tj, err := mu.Update(ctx, tjid, func (tj *tryjob.Tryjob) error {
//	  if tj.Result == nil {
//	    return ErrStopMutation // noop
//	  }
//	  tj.Result = nil
//	  return nil
//	})
//
//	if err != nil {
//	  return errors.Annotate(err, "failed to reset Result").Err()
//	}
//
// doSomething(ctx, tj)
// ```
var ErrStopMutation = errors.New("stop Tryjob mutation")

// MutateCallback is called by Mutator to mutate the Tryjob inside transactions.
//
// The function should be idempotent.
//
// If no error is returned, Mutator proceeds saving the Tryjob.
//
// If special ErrStopMutation is returned, Mutator aborts the transaction and
// returns existing Tryjob read from Datastore and no error. In the special
// case of Upsert(), the returned Tryjob may actually be nil if Tryjob didn't
// exist.
//
// If any error is returned other than ErrStopMutation, Mutator aborts the
// transaction and returns nil Tryjob and the exact same error.
type MutateCallback func(tj *Tryjob) error

// Upsert creates new or updates existing Tryjob via a dedicated transaction.
//
// Prefer to use Update if Tryjob ID is known.
//
// If Tryjob didn't exist before, the callback is provided a Tryjob with
// temporarily reserved ID. Until Upsert returns with success, this ID is not
// final, but it's fine to use it in other entities saved within the same
// transaction.
//
// If Tryjob didn't exist before and the callback returns ErrStopMutation, then
// Upsert returns (nil, nil).
func (m *Mutator) Upsert(ctx context.Context, eid ExternalID, clbk MutateCallback) (*Tryjob, error) {
	// Quick path in case Tryjob already exists, which is a common case,
	// and can usually be satisfied by dscache lookup.
	mapEntity := tryjobMap{ExternalID: eid}
	switch err := datastore.Get(ctx, &mapEntity); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		// OK, proceed to slow path below.
	case err != nil:
		return nil, errors.Annotate(err, "failed to get tryjobMap entity %q", eid).Tag(transient.Tag).Err()
	default:
		return m.Update(ctx, mapEntity.InternalID, clbk)
	}

	var result *Tryjob
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		// Check if Tryjob exists and prepare appropriate Tryjob mutation.
		var tjMutation *TryjobMutation
		mapEntity := tryjobMap{ExternalID: eid}
		switch err := datastore.Get(ctx, &mapEntity); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			tjMutation, err = m.beginInsert(ctx, eid)
			if err != nil {
				return err
			}
		case err != nil:
			return errors.Annotate(err, "failed to get tryjobMap entity %q", eid).Tag(transient.Tag).Err()
		default:
			if tjMutation, err = m.Begin(ctx, mapEntity.InternalID); err != nil {
				return err
			}
			result = tjMutation.Tryjob
		}
		if err := clbk(tjMutation.Tryjob); err != nil {
			return err
		}
		result, err = tjMutation.Finalize(ctx)
		return err
	}, nil)
	switch {
	case errors.Is(err, ErrStopMutation):
		return result, nil
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, errors.Annotate(err, "failed to commit Upsert of Tryjob %q", eid).Tag(transient.Tag).Err()
	default:
		return result, nil
	}
}

// Update mutates one Tryjob via a dedicated transaction.
//
// If the callback returns ErrStopMutation, then Update returns the read Tryjob
// entity and nil error.
func (m *Mutator) Update(ctx context.Context, id common.TryjobID, clbk MutateCallback) (*Tryjob, error) {
	var result *Tryjob
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		tjMutation, err := m.Begin(ctx, id)
		if err != nil {
			return err
		}
		result = tjMutation.Tryjob
		if err := clbk(tjMutation.Tryjob); err != nil {
			return err
		}
		result, err = tjMutation.Finalize(ctx)
		return err
	}, nil)
	switch {
	case errors.Is(err, ErrStopMutation):
		return result, nil
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, errors.Annotate(err, "failed to commit update on CL %d", id).Tag(transient.Tag).Err()
	default:
		return result, nil
	}
}

// TryjobMutation encapsulates one Tryjob mutation.
type TryjobMutation struct {
	// Tryjob can be modified except the following fields:
	//  * ID
	//  * ExternalID
	//  * EVersion
	//  * EntityCreateTime
	//  * EntityUpdateTime
	Tryjob *Tryjob

	// m is a back reference to its parent -- Mutator.
	m *Mutator

	// trans is only to detect incorrect usage.
	trans datastore.Transaction

	id         common.TryjobID
	externalID ExternalID

	priorEversion   int64
	priorCreateTime time.Time
	priorUpdateTime time.Time
}

func (m *Mutator) beginInsert(ctx context.Context, eid ExternalID) (*TryjobMutation, error) {
	tjMutation := &TryjobMutation{
		Tryjob: &Tryjob{ExternalID: eid},
		m:      m,
		trans:  datastore.CurrentTransaction(ctx),
	}
	if err := datastore.AllocateIDs(ctx, tjMutation.Tryjob); err != nil {
		return nil, errors.Annotate(err, "failed to allocate new Tryjob ID for %q", eid).Tag(transient.Tag).Err()
	}
	tjMap := &tryjobMap{ExternalID: eid, InternalID: tjMutation.Tryjob.ID}
	if err := datastore.Put(ctx, tjMap); err != nil {
		return nil, errors.Annotate(err, "failed to insert clMap entity for %q", eid).Tag(transient.Tag).Err()
	}
	tjMutation.backup()
	return tjMutation, nil
}

// Begin starts mutation of one Tryjob inside an existing transaction.
func (m *Mutator) Begin(ctx context.Context, id common.TryjobID) (*TryjobMutation, error) {
	tjMutation := &TryjobMutation{
		Tryjob: &Tryjob{ID: id},
		m:      m,
		trans:  datastore.CurrentTransaction(ctx),
	}
	if tjMutation.trans == nil {
		return nil, errors.New("tryjob.Mutator must be called inside an existing Datastore transaction")
	}
	switch err := datastore.Get(ctx, tjMutation.Tryjob); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, errors.Annotate(err, "Tryjob %d doesn't exist", id).Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to get Tryjob %d", id).Tag(transient.Tag).Err()
	}
	tjMutation.backup()
	return tjMutation, nil
}

func (tjm *TryjobMutation) backup() {
	tjm.id = tjm.Tryjob.ID
	tjm.externalID = tjm.Tryjob.ExternalID
	tjm.priorEversion = tjm.Tryjob.EVersion
	tjm.priorCreateTime = tjm.Tryjob.EntityCreateTime
	tjm.priorUpdateTime = tjm.Tryjob.EntityUpdateTime
}

// Finalize finalizes Tryjob mutation.
//
// Must be called at most once.
// Must be called in the same Datastore transaction as Begin() which began the
// Tryjob mutation.
func (tjm *TryjobMutation) Finalize(ctx context.Context) (*Tryjob, error) {
	tjm.finalize(ctx)
	if err := datastore.Put(ctx, tjm.Tryjob); err != nil {
		return nil, errors.Annotate(err, "failed to put Tryjob %d", tjm.id).Tag(transient.Tag).Err()
	}
	if err := tjm.m.notifyRuns(ctx, tjm); err != nil {
		return nil, err
	}
	return tjm.Tryjob, nil
}

func (tjm *TryjobMutation) finalize(ctx context.Context) {
	switch t := datastore.CurrentTransaction(ctx); {
	case tjm.trans == nil:
		panic(errors.New("tryjob.TryjobMutation.Finalize called the second time"))
	case t == nil:
		panic(errors.New("tryjob.TryjobMutation.Finalize must be called inside an existing Datastore transaction"))
	case t != tjm.trans:
		panic(errors.New("tryjob.TryjobMutation.Finalize called inside a different Datastore transaction"))
	}
	tjm.trans = nil

	switch {
	case tjm.id != tjm.Tryjob.ID:
		panic(errors.New("Tryjob.ID must not be modified"))
	case tjm.externalID != tjm.Tryjob.ExternalID:
		panic(errors.New("Tryjob.ExternalID must not be modified"))
	case tjm.priorEversion != tjm.Tryjob.EVersion:
		panic(fmt.Errorf("Tryjob.EVersion must not be modified"))
	case !tjm.priorCreateTime.Equal(tjm.Tryjob.EntityCreateTime):
		panic(fmt.Errorf("Tryjob.EntityCreateTime must not be modified"))
	case !tjm.priorUpdateTime.Equal(tjm.Tryjob.EntityUpdateTime):
		panic(fmt.Errorf("Tryjob.EntityUpdateTime must not be modified"))
	}
	tjm.Tryjob.EVersion++
	now := datastore.RoundTime(clock.Now(ctx).UTC())
	if tjm.Tryjob.EntityCreateTime.IsZero() {
		tjm.Tryjob.EntityCreateTime = now
	}
	tjm.Tryjob.EntityUpdateTime = now
}

// BeginBatch starts a batch of Tryjob mutations within the same Datastore
// transaction.
func (m *Mutator) BeginBatch(ctx context.Context, ids common.TryjobIDs) ([]*TryjobMutation, error) {
	trans := datastore.CurrentTransaction(ctx)
	if trans == nil {
		panic(fmt.Errorf("tryjob.Mutator.BeginBatch must be called inside an existing Datastore transaction"))
	}
	tryjobs, err := LoadTryjobsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	muts := make([]*TryjobMutation, len(ids))
	for i, tj := range tryjobs {
		muts[i] = &TryjobMutation{
			Tryjob: tj,
			m:      m,
			trans:  trans,
		}
		muts[i].backup()
	}
	return muts, nil
}

// FinalizeBatch finishes a batch of Tryjob mutations within the same Datastore
// transaction.
//
// The given mutations can originate from either Begin or BeginBatch calls.
// The only requirement is that they must all originate within the current
// Datastore transaction.
func (m *Mutator) FinalizeBatch(ctx context.Context, muts []*TryjobMutation) ([]*Tryjob, error) {
	tjs := make([]*Tryjob, len(muts))
	for i, mut := range muts {
		mut.finalize(ctx)
		tjs[i] = mut.Tryjob
	}
	if err := datastore.Put(ctx, tjs); err != nil {
		return nil, errors.Annotate(err, "failed to put %d Tryjobs", len(tjs)).Tag(transient.Tag).Err()
	}
	if err := m.notifyRuns(ctx, muts...); err != nil {
		return nil, err
	}
	return tjs, nil
}

// notifyRuns notifies the Runs interested in the given Tryjobs.
func (m *Mutator) notifyRuns(ctx context.Context, tjms ...*TryjobMutation) error {
	eventsByRun := make(map[common.RunID]*TryjobUpdatedEvents)
	for _, tjm := range tjms {
		for _, runID := range tjm.Tryjob.AllWatchingRuns() {
			if _, ok := eventsByRun[runID]; !ok {
				eventsByRun[runID] = &TryjobUpdatedEvents{}
			}
			eventsByRun[runID].Events = append(eventsByRun[runID].Events, &TryjobUpdatedEvent{
				TryjobId: int64(tjm.Tryjob.ID),
			})
		}
	}
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(8)
	for runID, events := range eventsByRun {
		eg.Go(func() error {
			return m.rm.NotifyTryjobsUpdated(ectx, runID, events)
		})
	}
	return eg.Wait()
}
