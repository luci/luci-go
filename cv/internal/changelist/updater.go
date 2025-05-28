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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/metrics"
)

const (
	// BatchUpdateCLTaskClass is the Task Class ID of the BatchUpdateCLTask,
	// which is enqueued only during a transaction.
	BatchUpdateCLTaskClass = "batch-update-cl"
	// UpdateCLTaskClass is the Task Class ID of the UpdateCLTask.
	UpdateCLTaskClass = "update-cl"

	// blindRefreshInterval sets interval between blind refreshes of a CL.
	blindRefreshInterval = time.Minute

	// knownRefreshInterval sets interval between refreshes of a CL when
	// updatedHint is known.
	knownRefreshInterval = 15 * time.Minute

	// autoRefreshAfter makes CLs worthy of "blind" refresh.
	//
	// "blind" refresh means that CL is already stored in Datastore and is up to
	// the date to the best knowledge of CV.
	autoRefreshAfter = 2 * time.Hour
)

// UpdaterBackend abstracts out fetching CL details from code review backend.
type UpdaterBackend interface {
	// Kind identifies the backend.
	//
	// It's also the first part of the CL's ExternalID, e.g. "gerrit".
	// Must not contain a slash.
	Kind() string

	// LookupApplicableConfig returns the latest ApplicableConfig for the previously
	// saved CL.
	//
	// See CL.ApplicableConfig field doc for more details. Roughly, it finds which
	// LUCI projects are configured to watch this CL.
	//
	// Updater calls LookupApplicableConfig() before Fetch() in order to avoid
	// the unnecessary Fetch() call entirely, e.g. if the CL is up to date or if
	// the CL is definitely not watched by a specific LUCI project.
	//
	// Returns non-nil ApplicableConfig normally.
	// Returns nil ApplicableConfig if the previously saved CL state isn't
	// sufficient to confidently determine the ApplicableConfig.
	LookupApplicableConfig(ctx context.Context, saved *CL) (*ApplicableConfig, error)

	// Fetch fetches the CL in the context of a given project.
	Fetch(ctx context.Context, input *FetchInput) (UpdateFields, error)

	// HasChanged decides whether the CL in the backend has changed from existing
	// snapshot in LUCI CV.
	HasChanged(cvCurrent, backendCurrent *Snapshot) bool

	// TQErrorSpec allows customizing logging and error TQ-specific handling.
	//
	// For example, Gerrit backend may wish to retry out of quota errors without
	// logging detailed stacktrace.
	TQErrorSpec() common.TQIfy
}

// FetchInput an input for UpdaterBackend.Fetch.
//
// It contains fields for what to fetch with meta information.
type FetchInput struct {
	// CL of the ChangeList to fetch a snapshot of.
	//
	// If CL.ID in the input is 0, it means the CL entity doesn't exist in
	// Datastore. The cl.ExternalID is always set.
	CL *CL
	// Project is the LUCI project to use the scoped account of for the fetch
	// operation to be performed.
	Project string
	// UpdatedHint, if not zero time, is the backend-originating timestamp of
	// the most recent CL update time. It's sourced by CV by e.g. polling or
	// PubSub subscription. It is useful to detect and work around backend's
	// eventual consistency.
	UpdatedHint time.Time
	// Requester identifies various scenarios that issued the Fetch invocation.
	Requester UpdateCLTask_Requester
	Hint      *UpdateCLTask_Hint
}

// NewFetchInput returns FetchInput for a given CL and UpdateCLTask.
func NewFetchInput(cl *CL, task *UpdateCLTask) *FetchInput {
	return &FetchInput{
		CL:        cl,
		Project:   task.GetLuciProject(),
		Hint:      task.GetHint(),
		Requester: task.GetRequester(),
	}
}

// UpdateFields defines what parts of CL to update.
//
// At least one field must be specified.
type UpdateFields struct {
	// Snapshot overwrites existing CL snapshot if newer according to its
	// .ExternalUpdateTime.
	Snapshot *Snapshot

	// ApplicableConfig overwrites existing CL ApplicableConfig if semantically
	// different from existing one.
	ApplicableConfig *ApplicableConfig

	// AddDependentMeta adds or overwrites metadata per LUCI project in CL AsDepMeta.
	// Doesn't affect metadata stored for projects not referenced here.
	AddDependentMeta *Access

	// DelAccess deletes Access records for the given projects.
	DelAccess []string
}

// IsEmpty returns true if no updates are necessary.
func (u UpdateFields) IsEmpty() bool {
	return (u.Snapshot == nil &&
		u.ApplicableConfig == nil &&
		len(u.AddDependentMeta.GetByProject()) == 0 &&
		len(u.DelAccess) == 0)
}

func (u UpdateFields) shouldUpdateSnapshot(cl *CL, backend UpdaterBackend) bool {
	switch {
	case u.Snapshot == nil:
		return false
	case cl.Snapshot == nil:
		return true
	case cl.Snapshot.GetOutdated() != nil:
		return true
	case cl.Snapshot.GetLuciProject() != u.Snapshot.GetLuciProject():
		return true
	case backend.HasChanged(cl.Snapshot, u.Snapshot):
		return true
	default:
		return false
	}
}

// Apply applies the UpdatedFields to a given CL.
func (u UpdateFields) Apply(cl *CL, backend UpdaterBackend) (changed, changedSnapshot bool) {
	if u.ApplicableConfig != nil && !cl.ApplicableConfig.SemanticallyEqual(u.ApplicableConfig) {
		cl.ApplicableConfig = u.ApplicableConfig
		changed = true
	}

	if u.shouldUpdateSnapshot(cl, backend) {
		cl.Snapshot = u.Snapshot
		changed, changedSnapshot = true, true
	}

	switch {
	case u.AddDependentMeta == nil:
	case cl.Access == nil || cl.Access.GetByProject() == nil:
		cl.Access = u.AddDependentMeta
		changed = true
	default:
		e := cl.Access.GetByProject()
		for lProject, v := range u.AddDependentMeta.GetByProject() {
			if v.GetNoAccessTime() == nil {
				panic("NoAccessTime must be set")
			}
			old, exists := e[lProject]
			if !exists || old.GetUpdateTime().AsTime().Before(v.GetUpdateTime().AsTime()) {
				if old.GetNoAccessTime() != nil && old.GetNoAccessTime().AsTime().Before(v.GetNoAccessTime().AsTime()) {
					v.NoAccessTime = old.NoAccessTime
				}
				e[lProject] = v
				changed = true
			}
		}
	}

	if len(u.DelAccess) > 0 && len(cl.Access.GetByProject()) > 0 {
		for _, p := range u.DelAccess {
			if _, exists := cl.Access.GetByProject()[p]; exists {
				changed = true
				delete(cl.Access.ByProject, p)
				if len(cl.Access.GetByProject()) == 0 {
					cl.Access = nil
					break
				}
			}
		}
	}

	return
}

// Updater knows how to update CLs from relevant backend (e.g. Gerrit),
// notifying other CV parts as needed.
type Updater struct {
	tqd     *tq.Dispatcher
	mutator *Mutator

	rwmutex  sync.RWMutex // guards `backends`
	backends map[string]UpdaterBackend
}

// NewUpdater creates a new Updater.
//
// Starts without backends, but they ought to be added via RegisterBackend().
func NewUpdater(tqd *tq.Dispatcher, m *Mutator) *Updater {
	u := &Updater{
		tqd:      tqd,
		mutator:  m,
		backends: make(map[string]UpdaterBackend, 1),
	}
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           BatchUpdateCLTaskClass,
		Prototype:    &BatchUpdateCLTask{},
		Queue:        "update-cl",
		Quiet:        true,
		QuietOnError: true,
		Kind:         tq.Transactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*BatchUpdateCLTask)
			err := u.handleBatch(ctx, t)
			return common.TQifyError(ctx, err)
		},
	})
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           UpdateCLTaskClass,
		Prototype:    &UpdateCLTask{},
		Queue:        "update-cl",
		Quiet:        true,
		QuietOnError: true,
		Kind:         tq.FollowsContext,
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*UpdateCLTask)
			// NOTE: unlike other TQ handlers code in CV, the common.TQifyError is
			// done inside the handler to allow per-backend definition of which errors
			// are retriable.
			return u.handleCL(ctx, t)
		},
	})
	return u
}

// RegisterBackend registers a backend.
//
// Panics if backend for the same kind is already registered.
func (u *Updater) RegisterBackend(b UpdaterBackend) {
	kind := b.Kind()
	if strings.ContainsRune(kind, '/') {
		panic(fmt.Errorf("backend %T of kind %q must not contain '/'", b, kind))
	}
	u.rwmutex.Lock()
	defer u.rwmutex.Unlock()
	if _, exists := u.backends[kind]; exists {
		panic(fmt.Errorf("backend %q is already registered", kind))
	}
	u.backends[kind] = b
}

// ScheduleBatch schedules update of several CLs.
//
// If called in a transaction, enqueues exactly one TQ task transactionally.
// This allows to write 1 Datastore entity during a transaction instead of N
// entities if Schedule() was used for each CL.
//
// Otherwise, enqueues 1 TQ task per CL non-transactionally and in parallel.
func (u *Updater) ScheduleBatch(ctx context.Context, luciProject string, cls []*CL, requester UpdateCLTask_Requester) error {
	tasks := make([]*UpdateCLTask, len(cls))
	for i, cl := range cls {
		tasks[i] = &UpdateCLTask{
			LuciProject: luciProject,
			ExternalId:  string(cl.ExternalID),
			Id:          int64(cl.ID),
			Requester:   requester,
		}
	}

	switch {
	case len(tasks) == 1:
		// Optimization for the most frequent use-case of single-CL Runs.
		return u.Schedule(ctx, tasks[0])
	case datastore.CurrentTransaction(ctx) == nil:
		return u.handleBatch(ctx, &BatchUpdateCLTask{Tasks: tasks})
	default:
		return u.tqd.AddTask(ctx, &tq.Task{
			Payload: &BatchUpdateCLTask{Tasks: tasks},
			Title:   fmt.Sprintf("batch-%s-%d", luciProject, len(tasks)),
		})
	}
}

// Schedule dispatches a TQ task. It should be used instead of the direct
// tq.AddTask to allow for consistent de-duplication.
func (u *Updater) Schedule(ctx context.Context, payload *UpdateCLTask) error {
	return u.ScheduleDelayed(ctx, payload, 0)
}

// ScheduleDelayed is the same as Schedule but with a delay.
func (u *Updater) ScheduleDelayed(ctx context.Context, payload *UpdateCLTask, delay time.Duration) error {
	task := &tq.Task{
		Payload: payload,
		Delay:   delay,
		Title:   makeTQTitleForHumans(payload),
	}
	if payload.Requester == UpdateCLTask_REQUESTER_CLASS_UNSPECIFIED {
		return fmt.Errorf("bug: UpdateCLTask.Requester unspecified: %s", payload)
	}
	if datastore.CurrentTransaction(ctx) == nil {
		task.DeduplicationKey = makeTaskDeduplicationKey(ctx, payload, delay)
	}
	return u.tqd.AddTask(ctx, task)
}

// ResolveAndScheduleDepsUpdate resolves deps, creating new CL entities as
// necessary, and schedules an update task for each dep which needs an update.
//
// It's meant to be used by the Updater backends.
//
// Returns a sorted slice of Deps by their CL ID, ready to be stored as
// CL.Snapshot.Deps.
func (u *Updater) ResolveAndScheduleDepsUpdate(ctx context.Context, luciProject string, deps map[ExternalID]DepKind, requester UpdateCLTask_Requester) ([]*Dep, error) {
	// Optimize for the most frequent case whereby deps are already known to CV
	// and were updated recently enough that no task scheduling is even necessary.

	// Batch-resolve external IDs to CLIDs, and load all existing CLs.
	resolvingDeps, err := resolveDeps(ctx, luciProject, deps)
	if err != nil {
		return nil, err
	}
	// Identify indexes of deps which need to have an update task scheduled.
	ret := make([]*Dep, len(deps))
	var toSchedule []int // indexes
	for i, d := range resolvingDeps {
		if d.ready {
			ret[i] = d.resolvedDep
		} else {
			// Also covers the case of a dep not yet having a CL entity.
			toSchedule = append(toSchedule, i)
		}
	}
	if len(toSchedule) == 0 {
		// Quick path exit.
		return sortDeps(ret), nil
	}

	errs := parallel.WorkPool(min(10, len(toSchedule)), func(work chan<- func() error) {
		for _, i := range toSchedule {
			d := resolvingDeps[i]
			work <- func() error {
				if err := d.createIfNotExists(ctx, u.mutator, luciProject); err != nil {
					return err
				}
				if err := d.schedule(ctx, u, luciProject, requester); err != nil {
					return err
				}
				ret[i] = d.resolvedDep
				return nil
			}
		}
	})
	if errs != nil {
		return nil, common.MostSevereError(err)
	}
	return sortDeps(ret), nil
}

///////////////////////////////////////////////////////////////////////////////
// implementation details.

func (u *Updater) handleBatch(ctx context.Context, batch *BatchUpdateCLTask) error {
	total := len(batch.GetTasks())
	err := parallel.WorkPool(min(16, total), func(work chan<- func() error) {
		for _, task := range batch.GetTasks() {
			work <- func() error { return u.Schedule(ctx, task) }
		}
	})
	switch merrs, ok := err.(errors.MultiError); {
	case err == nil:
		return nil
	case !ok:
		return err
	default:
		failed, _ := merrs.Summary()
		err = common.MostSevereError(merrs)
		return errors.Fmt("failed to schedule UpdateCLTask for %d out of %d CLs, keeping the most severe error: %w", failed, total, err)
	}
}

// TestingForceUpdate runs the CL Updater synchronously.
//
// For use in tests only. Production code should use Schedule() to benefit from
// task de-duplication.
//
// TODO(crbug/1284393): revisit the usefulness of the sync refresh after
// consistency-on-demand is provided by Gerrit.
func (u *Updater) TestingForceUpdate(ctx context.Context, task *UpdateCLTask) error {
	return u.handleCL(ctx, task)
}

func (u *Updater) handleCL(ctx context.Context, task *UpdateCLTask) error {
	cl, err := u.preload(ctx, task)
	if err != nil {
		return common.TQifyError(ctx, err)
	}
	// cl.ID == 0 means CL doesn't yet exist.
	ctx = logging.SetFields(ctx, logging.Fields{
		"project": task.GetLuciProject(),
		"id":      cl.ID,
		"eid":     cl.ExternalID,
	})

	backend, err := u.backendFor(cl)
	if err != nil {
		return common.TQifyError(ctx, err)
	}

	switch err := u.handleCLWithBackend(ctx, task, cl, backend); {
	case err == errHackRetryForOutOfQuota:
		return tq.Ignore.Apply(err)
	case err != nil:
		return backend.TQErrorSpec().Error(ctx, err)
	}
	return nil
}

var errHackRetryForOutOfQuota = errors.New("hack retry for out of quota")

func (u *Updater) handleCLWithBackend(ctx context.Context, task *UpdateCLTask, cl *CL, backend UpdaterBackend) error {
	// Save ID and ExternalID before giving CL to backend to avoid accidental corruption.
	id, eid := cl.ID, cl.ExternalID
	skip, updateFields, err := u.trySkippingFetch(ctx, task, cl, backend)
	var fetchDuration time.Duration
	switch {
	case err != nil:
		return err
	case !skip:
		now := clock.Now(ctx)
		updateFields, err = backend.Fetch(ctx, NewFetchInput(cl, task))
		fetchDuration = clock.Since(ctx, now)
		switch {
		case err != nil && errors.Unwrap(err) == gerrit.ErrOutOfQuota && task.GetLuciProject() == "chromeos":
			// HACK: don't retry on out of quota error, instead schedule another task
			// with delay so that it will be deduplicated in cloud task with any
			// subsequent tasks.
			if scheduleErr := u.ScheduleDelayed(ctx, task, blindRefreshInterval); scheduleErr != nil {
				return errors.Fmt("%T.Fetch failed: %w", backend, err)
			}
			return errHackRetryForOutOfQuota
		case err != nil:
			return errors.Fmt("%T.Fetch failed: %w", backend, err)
		}
	}

	if updateFields.IsEmpty() {
		logging.Debugf(ctx, "No update is necessary")
		return nil
	}

	// Transactionally update the CL.
	var changed, changedSnapshot bool
	transClbk := func(latest *CL) error {
		if changed, changedSnapshot = updateFields.Apply(latest, backend); !changed {
			// Someone, possibly even us in case of Datastore transaction retry, has
			// already updated this CL.
			return ErrStopMutation
		}
		return nil
	}
	if cl.ID == 0 {
		_, err = u.mutator.Upsert(ctx, task.GetLuciProject(), eid, transClbk)
	} else {
		_, err = u.mutator.Update(ctx, task.GetLuciProject(), id, transClbk)
	}

	if err != nil {
		return err
	}

	switch {
	case updateFields.Snapshot == nil:
		// Skip reporting the fetch metrics. It's either the fetch operation
		// failed or skipped.
	case skip:
		// Fetch was not performed; skip reporting the metrics.
	case changed:
		// Report the latency metrics only if the fetch actually returned
		// new data. If the data was the same as the existing snapshot,
		// the fetch wasn't needed, indeed.
		delay := clock.Now(ctx).Sub(updateFields.Snapshot.ExternalUpdateTime.AsTime())
		if delay < 0 {
			logging.Errorf(ctx, "negative CL fetch duration (%d) detected", delay)
			delay = 0
		}
		metrics.Internal.CLIngestionLatency.Add(
			ctx, delay.Seconds(), task.GetRequester().String(), task.GetIsForDep(),
			task.GetLuciProject(), changedSnapshot)
		metrics.Internal.CLIngestionLatencyWithoutFetch.Add(
			ctx, (delay - fetchDuration).Seconds(), task.GetRequester().String(),
			task.GetIsForDep(), task.GetLuciProject(), changedSnapshot)
		fallthrough
	default:
		metrics.Internal.CLIngestionAttempted.Add(
			ctx, 1, task.GetRequester().String(), changed, task.GetIsForDep(),
			task.GetLuciProject(), changedSnapshot)
	}
	return nil
}

// trySkippingFetch checks if a fetch from the backend can be skipped.
//
// Returns true if so.
// NOTE: UpdateFields may be set if fetch can be skipped, meaning CL entity
// should be updated in Datastore.
func (u *Updater) trySkippingFetch(ctx context.Context, task *UpdateCLTask, cl *CL, backend UpdaterBackend) (bool, UpdateFields, error) {
	if cl.ID == 0 || cl.Snapshot == nil || cl.Snapshot.GetOutdated() != nil {
		return false, UpdateFields{}, nil
	}

	hintedTS := task.GetHint().GetExternalUpdateTime()
	hintedRevID := task.GetHint().GetMetaRevId()
	switch {
	case hintedTS == nil && hintedRevID == "":
		// fetch always if there is no hint available.
		return false, UpdateFields{}, nil
	case hintedRevID != "" && hintedRevID != cl.Snapshot.GetGerrit().GetInfo().GetMetaRevId():
		// fetch always if MetaRev is different to the rev id of the stored
		// snapshot. If the fetched snapshot is older than the stored snapshot,
		// it will be skipped to update the DS entity with the fetched snapshot.
		return false, UpdateFields{}, nil
	case hintedTS != nil && hintedTS.AsTime().After(cl.Snapshot.GetExternalUpdateTime().AsTime()):
		// There is no confidence that Snapshot is up-to-date, so proceed fetching
		// anyway.

		// NOTE: it's tempting to check first whether the LUCI project is watching
		// the CL given the existing Snapshot and skip the fetch if it's not the
		// case. However, for Gerrit CLs, the ref is mutable after the CL
		// creation and since ref is used to determine if CL is being watched,
		// we can't skip the fetch. For an example, see Gerrit move API
		// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#move-change
		return false, UpdateFields{}, nil
	}

	// CL Snapshot is up to date, but does it belong to the right LUCI project?
	acfg, err := backend.LookupApplicableConfig(ctx, cl)
	if err != nil {
		err = errors.Fmt("%T.LookupApplicableConfig failed: %w", backend, err)
		return false, UpdateFields{}, err
	}
	if acfg == nil {
		// Insufficient saved CL, need to fetch before deciding if CL is watched.
		return false, UpdateFields{}, err
	}

	// Update CL with the new set of watching projects if materially different,
	// which should be saved to Datastore even if the fetch from Gerrit itself is
	// skipped.
	var toUpdate UpdateFields
	if !cl.ApplicableConfig.SemanticallyEqual(acfg) {
		toUpdate.ApplicableConfig = acfg
	}

	if !acfg.HasProject(task.GetLuciProject()) {
		// This project isn't watching the CL, so no need to fetch.
		//
		// NOTE: even if the Snapshot was fetched in the context of this project before,
		// we don't have to erase the Snapshot from the CL immediately: the update
		// in cl.ApplicableConfig suffices to ensure that CV won't be using the
		// Snapshot.
		return true, toUpdate, nil
	}

	if !acfg.HasProject(cl.Snapshot.GetLuciProject()) {
		// The Snapshot was previously fetched in the context of a project which is
		// no longer watching the CL.
		//
		// This can happen in practice in case of e.g. newly created "chromium-mXXX"
		// project to watch for a specific ref which was previously watched by a
		// generic "chromium" project. A Snapshot of a CL on such a ref would have
		// been fetched in the context of "chromium" first, and now it must be re-fetched
		// under "chromium-mXXX" to verify that the new project hasn't lost access
		// to the Gerrit CL.
		logging.Warningf(ctx, "Detected switch from %q LUCI project", cl.Snapshot.GetLuciProject())
		return false, toUpdate, nil
	}

	// At this point, these must be true:
	// * the Snapshot is up-to-date to the best of CV knowledge;
	// * this project is watching the CL, but there may be other projects, too;
	// * the Snapshot was created by a project still watching the CL, but which may
	//   differ from this project.
	if len(acfg.GetProjects()) >= 2 {
		// When there are several watching projects, projects shouldn't race
		// re-fetching & saving Snapshot. No new Runs are going to be started on
		// such CLs, so skip fetching new snapshot.
		return true, toUpdate, nil
	}

	// There is just 1 project, so check the invariant.
	if task.GetLuciProject() != cl.Snapshot.GetLuciProject() {
		panic(fmt.Errorf("BUG: this project %q must have created the Snapshot, not %q", task.GetLuciProject(), cl.Snapshot.GetLuciProject()))
	}

	if restriction := cl.Access.GetByProject()[task.GetLuciProject()]; restriction != nil {
		// For example, Gerrit has responded HTTP 403/404 before.
		// Must fetch again to verify if restriction still holds.
		logging.Debugf(ctx, "Detected prior access restriction: %s", restriction)
		return false, toUpdate, nil
	}

	// Finally, do refresh if the CL entity is just really old and the meta rev
	// id is unset.
	switch {
	case hintedRevID != "":
	// skip the fetch if the meta rev id is the same as the rev id of the stored
	// snapshot.
	case clock.Since(ctx, cl.UpdateTime) > autoRefreshAfter:
		// Strictly speaking, cl.UpdateTime isn't just changed on refresh, but
		// also whenever Run starts/ends. However, the start of Run is usually
		// happenening right after recent refresh, and end of Run is usually
		// followed by the refresh.
		return false, toUpdate, nil
	}

	// OK, skip the fetch.
	return true, toUpdate, nil
}

func (*Updater) preload(ctx context.Context, task *UpdateCLTask) (*CL, error) {
	if task.GetLuciProject() == "" {
		return nil, errors.New("invalid task input: LUCI project must be given")
	}
	eid := ExternalID(task.GetExternalId())
	id := common.CLID(task.GetId())
	switch {
	case id != 0:
		cl := &CL{ID: common.CLID(id)}
		switch err := datastore.Get(ctx, cl); {
		case err == datastore.ErrNoSuchEntity:
			return nil, errors.Fmt("CL %d %q doesn't exist in Datastore: %w", id, task.GetExternalId(), err)
		case err != nil:
			return nil, transient.Tag.Apply(errors.Fmt("failed to load CL %d: %w", id, err))
		case eid != "" && eid != cl.ExternalID:
			return nil, errors.Fmt("invalid task input: CL %d actually has %q ExternalID, not %q", id, cl.ExternalID, eid)
		default:
			return cl, nil
		}
	case eid == "":
		return nil, errors.New("invalid task input: either internal ID or ExternalID must be given")
	default:
		switch cl, err := eid.Load(ctx); {
		case err != nil:
			return nil, transient.Tag.Apply(errors.Fmt("failed to load CL %q: %w", eid, err))
		case cl == nil:
			// New CL to be created.
			return &CL{
				ExternalID: eid,
				ID:         0, // will be populated later.
				EVersion:   0,
			}, nil
		default:
			return cl, nil
		}
	}
}

func (u *Updater) backendFor(cl *CL) (UpdaterBackend, error) {
	kind, err := cl.ExternalID.kind()
	if err != nil {
		return nil, err
	}
	u.rwmutex.RLock()
	defer u.rwmutex.RUnlock()
	if b, exists := u.backends[kind]; exists {
		return b, nil
	}
	return nil, errors.Fmt("%q backend is not supported", kind)
}

// makeTaskDeduplicationKey returns TQ task deduplication key.
func makeTaskDeduplicationKey(ctx context.Context, t *UpdateCLTask, delay time.Duration) string {
	var sb strings.Builder
	sb.WriteString("v0")
	sb.WriteRune('\n')
	sb.WriteString(t.GetLuciProject())
	sb.WriteRune('\n')

	// Prefer ExternalID if both ID and ExternalID are known, as the most frequent
	// use-case for update via PubSub/Polling, which specifies ExternalID and may
	// not resolve it to internal ID just yet.
	uniqArg := t.GetExternalId()
	if uniqArg == "" {
		uniqArg = strconv.FormatInt(t.GetId(), 16)
	}
	sb.WriteString(uniqArg)

	// If the meta rev ID is set, dedup with a time window isn't necessary.
	// 1) Gerrit guarantees one publish for each of CL update events.
	// 2) # of redelivered messages should be low enough to ignore.
	// 3) If the same message is redelivered multiple times, the backend
	// will skip fetching the snapshot after the first message.
	// 4) If it's concerned that retries can fast burn out Gerrit quota,
	// pubsub retry config should be tuned, instead.
	if revID := t.GetHint().GetMetaRevId(); revID != "" {
		_, _ = fmt.Fprintf(&sb, "\n%s", revID)
		return sb.String()
	}

	// Dedup in the short term to avoid excessive number of refreshes,
	// but ensure eventually calling Schedule with the same payload results in a
	// new task. This is done by de-duping only within a single "epoch" window,
	// which differs by CL to avoid synchronized herd of requests hitting
	// a backend (e.g. Gerrit).
	//
	// +----------------------------------------------------------------------+
	// |                 ... -> time goes forward -> ....                     |
	// +----------------------------------------------------------------------+
	// |                                                                      |
	// | ... | epoch (N-1, CL-A) | epoch (N, CL-A) | epoch (N+1, CL-A) | ...  |
	// |                                                                      |
	// |            ... | epoch (N-1, CL-B) | epoch (N, CL-B) | ...           |
	// +----------------------------------------------------------------------+
	//
	// Furthermore, de-dup window differs based on whether updatedHint is given
	// or it's a blind refresh.
	interval := blindRefreshInterval
	if t.GetHint().GetExternalUpdateTime() != nil {
		interval = knownRefreshInterval
	}
	epochOffset := common.DistributeOffset(interval, "update-cl", t.GetLuciProject(), uniqArg)
	epochTS := clock.Now(ctx).Add(delay).Truncate(interval).Add(interval + epochOffset)
	_, _ = fmt.Fprintf(&sb, "\n%x", epochTS.UnixNano())
	if h := t.GetHint().GetExternalUpdateTime(); h != nil {
		_, _ = fmt.Fprintf(&sb, "\n%x", h.AsTime().UnixNano())
	}
	return sb.String()
}

// makeTQTitleForHumans makes human-readable TQ task title.
//
// WARNING: do not use for anything else. Doesn't guarantee uniqueness.
//
// It will be visible in logs as the suffix of URL in Cloud Tasks console and
// in the GAE requests log.
//
// The primary purpose is that quick search for specific CL in the GAE request
// log alone, as opposed to searching through much larger and separate stderr
// log of the process (which is where logging.Logf calls go into).
//
// For example,
//
//	"proj/gerrit/chromium/1111111/u2016-02-03T04:05:06Z/deadbeef"
//	"proj/gerrit/chromium/1111111/u2016-02-03T04:05:06Z"
//	"proj/gerrit/chromium/1111111/deadbeef"
func makeTQTitleForHumans(t *UpdateCLTask) string {
	var sb strings.Builder
	sb.WriteString(t.GetLuciProject())
	if id := t.GetId(); id != 0 {
		_, _ = fmt.Fprintf(&sb, "/%d", id)
	}
	if eid := t.GetExternalId(); eid != "" {
		sb.WriteRune('/')
		// Reduce verbosity in common case of Gerrit on googlesource.
		// Although it's possible to delegate this to backend, the additional
		// boilerplate isn't yet justified.
		if kind, err := ExternalID(eid).kind(); err == nil && kind == "gerrit" {
			eid = strings.Replace(eid, "-review.googlesource.com/", "/", 1)
		}
		sb.WriteString(eid)
	}
	if hintedTS := t.GetHint().GetExternalUpdateTime(); hintedTS != nil {
		sb.WriteString("/u")
		sb.WriteString(hintedTS.AsTime().UTC().Format(time.RFC3339))
	}
	if hintedRevID := t.GetHint().GetMetaRevId(); hintedRevID != "" {
		sb.WriteString("/")
		sb.WriteString(hintedRevID)
	}
	return sb.String()
}

const maxDepsLoadingBatchSize = 100

func resolveDeps(ctx context.Context, luciProject string, deps map[ExternalID]DepKind) ([]resolvingDep, error) {
	eids := make([]ExternalID, 0, len(deps))
	ret := make([]resolvingDep, 0, len(deps))
	for eid, kind := range deps {
		eids = append(eids, eid)
		ret = append(ret, resolvingDep{eid: eid, kind: kind})
	}

	ids, err := Lookup(ctx, eids)
	if err != nil {
		return nil, err
	}
	depCLs := make([]CL, 0, maxDepsLoadingBatchSize)
	depCLIndices := make([]int, 0, maxDepsLoadingBatchSize)
	for i, id := range ids {
		if id > 0 {
			cl := CL{ID: id}
			depCLs = append(depCLs, cl)
			depCLIndices = append(depCLIndices, i)
			ret[i].resolvedDep = &Dep{Clid: int64(id), Kind: ret[i].kind}
		}
		if len(depCLs) == maxDepsLoadingBatchSize || (len(depCLs) > 0 && i == len(ids)-1) {
			// cut a batch if max is reached or end of ids.
			if err := datastore.Get(ctx, depCLs); err != nil {
				// Mark error as transient because by this time, all CLIDs should have
				// corresponding CL entities in datastore.
				return nil, transient.Tag.Apply(errors.Fmt("failed to load %d CLs: %w", len(depCLs), err))
			}
			for j, depCL := range depCLs {
				ret[depCLIndices[j]].ready = !depNeedsRefresh(ctx, depCL, luciProject)
			}
			depCLs = depCLs[:0]
			depCLIndices = depCLIndices[:0]
		}
	}
	return ret, nil
}

// resolvingDep represents a dependency known by its external ID only being
// resolved.
//
// Helper struct for the Updater.ResolveAndScheduleDeps.
type resolvingDep struct {
	eid         ExternalID
	kind        DepKind
	ready       bool // true if already up to date and .dep is populated.
	resolvedDep *Dep // if nil, use createIfNotExists() to populate
}

func (d *resolvingDep) createIfNotExists(ctx context.Context, m *Mutator, luciProject string) error {
	if d.resolvedDep != nil {
		return nil // already exists
	}
	cl, err := m.Upsert(ctx, luciProject, d.eid, func(cl *CL) error {
		// TODO: somehow record when CL was inserted to put a boundary on how long
		// Project Manager should be waiting for the dep to be actually fetched &
		// its entity updated in Datastore.
		if cl.EVersion > 0 {
			// If CL already exists, we don't need to modify it % above comment.
			return ErrStopMutation
		}
		return nil
	})
	if err != nil {
		return err
	}
	d.resolvedDep = &Dep{Clid: int64(cl.ID), Kind: d.kind}
	return nil
}

func (d *resolvingDep) schedule(ctx context.Context, u *Updater, luciProject string, requester UpdateCLTask_Requester) error {
	return u.Schedule(ctx, &UpdateCLTask{
		ExternalId:  string(d.eid),
		Id:          d.resolvedDep.GetClid(),
		LuciProject: luciProject,
		Requester:   requester,
		IsForDep:    true,
	})
}

// sortDeps sorts given slice by CLID ASC in place and returns it.
func sortDeps(deps []*Dep) []*Dep {
	sort.Slice(deps, func(i, j int) bool {
		return deps[i].GetClid() < deps[j].GetClid()
	})
	return deps
}

// depNeedsRefresh returns true if the dependency CL needs a refresh in the
// context of a specific LUCI project.
func depNeedsRefresh(ctx context.Context, dep CL, luciProject string) bool {
	switch {
	case dep.Snapshot == nil:
		return true
	case dep.Snapshot.GetOutdated() != nil:
		return true
	case dep.Snapshot.GetLuciProject() != luciProject:
		return true
	default:
		return false
	}
}
