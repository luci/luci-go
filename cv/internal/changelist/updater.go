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
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
)

const (
	// BatchCLUpdateTaskClass is the Task Class ID of the BatchUpdateCLTask,
	// which is enqueued only during a transaction.
	BatchUpdateCLTaskClass = "batch-update-cl"
	// CLUpdateTaskClass is the Task Class ID of the UpdateCLTask.
	UpdateCLTaskClass = "update-cl"

	// blindRefreshInterval sets interval between blind refreshes of a CL.
	blindRefreshInterval = time.Minute

	// knownRefreshInterval sets interval between refreshes of a CL when
	// updatedHint is known.
	knownRefreshInterval = 15 * time.Minute
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
	//
	// If a given cl.ID is 0, it means the CL entity doesn't exist in Datastore.
	// The cl.ExternalID is always set.
	//
	// UpdatedHint, if not zero time, is the backend-originating timestamp of the
	// most recent CL update time. It's sourced by CV by e.g. polling or PubSub
	// subscription. It is useful to detect and work around backend's eventual
	// consistency.
	Fetch(ctx context.Context, cl *CL, luciProject string, updatedHint time.Time) (UpdateFields, error)

	// TQErrorSpec allows customizing logging and error TQ-specific handling.
	//
	// For example, Gerrit backend may wish to retry out of quota errors without
	// logging detailed stacktrace.
	TQErrorSpec() common.TQIfy
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

func (u UpdateFields) shouldUpdateSnapshot(cl *CL) bool {
	switch {
	case u.Snapshot == nil:
		return false
	case cl.Snapshot == nil:
		return true
	case cl.Snapshot.GetOutdated() != nil:
		return true
	case u.Snapshot.GetExternalUpdateTime().AsTime().After(cl.Snapshot.GetExternalUpdateTime().AsTime()):
		return true
	case cl.Snapshot.GetLuciProject() != u.Snapshot.GetLuciProject():
		return true
	default:
		return false
	}
}

func (u UpdateFields) Apply(cl *CL) (changed bool) {
	if u.ApplicableConfig != nil && !cl.ApplicableConfig.SemanticallyEqual(u.ApplicableConfig) {
		cl.ApplicableConfig = u.ApplicableConfig
		changed = true
	}

	if u.shouldUpdateSnapshot(cl) {
		cl.Snapshot = u.Snapshot
		changed = true
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
func (u *Updater) ScheduleBatch(ctx context.Context, luciProject string, cls []*CL) error {
	tasks := make([]*UpdateCLTask, len(cls))
	for i, cl := range cls {
		tasks[i] = &UpdateCLTask{
			LuciProject: luciProject,
			ExternalId:  string(cl.ExternalID),
			Id:          int64(cl.ID),
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
	if datastore.CurrentTransaction(ctx) == nil {
		task.DeduplicationKey = makeTaskDeduplicationKey(ctx, payload, delay)
	}
	return u.tqd.AddTask(ctx, task)
}

///////////////////////////////////////////////////////////////////////////////
// implementation details.

func (u *Updater) handleBatch(ctx context.Context, batch *BatchUpdateCLTask) error {
	total := len(batch.GetTasks())
	err := parallel.WorkPool(min(16, total), func(work chan<- func() error) {
		for _, task := range batch.GetTasks() {
			task := task
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
		return errors.Annotate(err, "failed to schedule UpdateCLTask for %d out of %d CLs, keeping the most severe error", failed, total).Err()
	}
}

func (u *Updater) handleCL(ctx context.Context, task *UpdateCLTask) error {
	ctx = logging.SetField(ctx, "project", task.GetLuciProject())
	// TODO(tandrii): implement.
	// TODO(tandrii): use backend-provided TQIfy spec.
	return common.TQifyError(ctx, nil)
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
	return nil, errors.Reason("%q backend is not supported", kind).Err()
}

// makeTaskDeduplicationKey returns TQ task deduplication key.
func makeTaskDeduplicationKey(ctx context.Context, t *UpdateCLTask, delay time.Duration) string {
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
	// Furthermore, de-dup window differs based on wheter updatedHint is given
	// or it's a blind refresh.
	interval := blindRefreshInterval
	if t.GetUpdatedHint() != nil {
		interval = knownRefreshInterval
	}
	// Prefer ExternalID if both ID and ExternalID are known, as the most frequent
	// use-case for update via PubSub/Polling, which specifies ExternalID and may
	// not resolve it to internal ID just yet.
	uniqArg := t.GetExternalId()
	if uniqArg == "" {
		uniqArg = strconv.FormatInt(t.GetId(), 16)
	}
	epochOffset := common.DistributeOffset(interval, "update-cl", t.GetLuciProject(), uniqArg)
	epochTS := clock.Now(ctx).Add(delay).Truncate(interval).Add(interval + epochOffset)

	var sb strings.Builder
	sb.WriteString("v0")
	sb.WriteRune('\n')
	sb.WriteString(t.GetLuciProject())
	sb.WriteRune('\n')
	sb.WriteString(uniqArg)
	_, _ = fmt.Fprintf(&sb, "\n%x", epochTS.UnixNano())
	if h := t.GetUpdatedHint(); h != nil {
		_, _ = fmt.Fprintf(&sb, "\n%x", h.AsTime().UnixNano())
	}
	return sb.String()
}

// makeTQTitleForHumans makes human-readable TQ task title.
//
// WARNING: do not use for anything else. Doesn't guarantee uniqueness.
//
// It'll be visible in logs as the suffix of URL in Cloud Tasks console
// and in the GAE requests log.
//
// The primary purpose is that quick search for specific CL in the GAE request
// log alone, as opposed to searching through much larger and separate stderr
// log of the process (which is where logging.Logf calls go into).
//
// For example, the title for the task with all the field specified:
//   "proj/gerrit/chromium/1111111/u2016-02-03T04:05:06Z"
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
	if h := t.GetUpdatedHint(); h != nil {
		sb.WriteString("/u")
		sb.WriteString(h.AsTime().UTC().Format(time.RFC3339))
	}
	return sb.String()
}
