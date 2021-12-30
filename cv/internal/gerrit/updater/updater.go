// Copyright 2020 The LUCI Authors.
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

package updater

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
)

const (
	TaskClass      = "refresh-gerrit-cl"
	TaskClassBatch = "batch-refresh-gerrit-cl"

	// blindRefreshInterval sets interval between blind refreshes of a Gerrit CL.
	blindRefreshInterval = time.Minute

	// knownRefreshInterval sets interval between refreshes of a Gerrit CL when
	// updatedHint is known.
	knownRefreshInterval = 15 * time.Minute
)

// Updater fetches Gerrit Change details and stores them as CV CLs in Datastore.
type Updater struct {
	gFactory  gerrit.Factory
	clMutator *changelist.Mutator
	tqd       *tq.Dispatcher
}

// New creates a new Updater.
func New(tqd *tq.Dispatcher, g gerrit.Factory, m *changelist.Mutator) *Updater {
	u := &Updater{g, m, tqd}
	// TODO(tandrii): remove these TQ classes after migrating to
	// changelist.Updater.
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           TaskClass,
		Prototype:    &RefreshGerritCL{},
		Queue:        "refresh-gerrit-cl",
		Quiet:        true,
		QuietOnError: true,
		Kind:         tq.FollowsContext,
		Handler: func(ctx context.Context, payload proto.Message) error {
			// Keep this function small, as it's not unit tested.
			t := payload.(*RefreshGerritCL)
			ctx = logging.SetField(ctx, "project", t.GetLuciProject())
			err := u.Refresh(ctx, t)
			return common.TQIfy{
				// Don't log the entire stack trace of stale data, which is sadly an
				// hourly occurrence.
				KnownRetry: []error{gerrit.ErrStaleData, gerrit.ErrOutOfQuota, gerrit.ErrGerritDeadlineExceeded},
			}.Error(ctx, err)
		},
	})
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           TaskClassBatch,
		Prototype:    &BatchRefreshGerritCL{},
		Queue:        "refresh-gerrit-cl",
		Quiet:        true,
		QuietOnError: true,
		Kind:         tq.Transactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			// Keep this function small, as it's not unit tested.
			t := payload.(*BatchRefreshGerritCL)
			err := u.RefreshBatch(ctx, t)
			return common.TQifyError(ctx, err)
		},
	})
	return u
}

// Schedule enqueues a TQ task to refresh a Gerrit CL as soon as possible.
//
// It should be used instead of directly using tq.AddTask, for consistent
// deduplication and ease of debugging.
func (u *Updater) Schedule(ctx context.Context, p *RefreshGerritCL) error {
	return u.ScheduleDelayed(ctx, p, 0)
}

// Schedule enqueues a TQ task to refresh a Gerrit CL after a given delay.
//
// It should be used instead of directly using tq.AddTask, for consistent
// deduplication and ease of debugging.
func (u *Updater) ScheduleDelayed(ctx context.Context, p *RefreshGerritCL, delay time.Duration) error {
	task := &tq.Task{
		Payload: p,
		Delay:   delay,
		Title:   fmt.Sprintf("%s/%s/%d", p.GetLuciProject(), p.GetHost(), p.GetChange()),
	}
	if clid := p.GetClidHint(); clid != 0 {
		task.Title += fmt.Sprintf("/clid-%d", clid)
	}
	var updatedHint time.Time
	if t := p.GetUpdatedHint(); t != nil {
		updatedHint = t.AsTime().UTC()
		task.Title += fmt.Sprintf("/u-%s", updatedHint.Format(time.RFC3339))
	}

	// If done within transaction or if must notify PM, can't use de-dup.
	if datastore.CurrentTransaction(ctx) == nil {
		// De-dup in the short term to avoid excessive number of refreshes, but
		// ensure eventually calling Schedule with the same payload results in
		// a new task. This is done by de-duping only within a single "epoch"
		// window, which differs by CL to avoid synchronized herd of requests
		// hitting Gerrit.
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
		// Furthermore, de-dup window differs based on whether updatedHint is given,
		// or whether it's a blind refresh.
		interval := blindRefreshInterval
		if updatedHint.IsZero() {
			interval = knownRefreshInterval
		}
		changeInHex := strconv.FormatInt(p.GetChange(), 16)
		epochOffset := common.DistributeOffset(interval, "refresh-gerrit-cl", p.GetLuciProject(), p.GetHost(), changeInHex)
		epochTS := clock.Now(ctx).Add(delay).Truncate(interval).Add(interval + epochOffset)

		u := updatedHint
		if updatedHint.IsZero() {
			u = time.Unix(0, 0)
		}
		task.DeduplicationKey = strings.Join([]string{
			"v0",
			p.GetLuciProject(),
			p.GetHost(),
			changeInHex,
			strconv.FormatInt(epochTS.UnixNano(), 16),
			strconv.FormatInt(u.UnixNano(), 16),
		}, "\n")
	}
	return u.tqd.AddTask(ctx, task)
}

// Refresh fetches the latest info from Gerrit.
//
// If datastore already contains snapshot with Gerrit-reported update time equal
// to or after updatedHint, then no updating or querying will be performed.
//
// Prefer Schedule() instead of Refresh() in production.
func (u *Updater) Refresh(ctx context.Context, r *RefreshGerritCL) (err error) {
	f := fetcher{
		gFactory:        u.gFactory,
		clMutator:       u.clMutator,
		scheduleRefresh: u.ScheduleDelayed,

		luciProject: r.GetLuciProject(),
		host:        r.GetHost(),
		change:      r.GetChange(),
	}
	if uh := r.GetUpdatedHint(); uh != nil {
		f.updatedHint = uh.AsTime()
	}
	defer func() { err = errors.Annotate(err, "failed to refresh %s", &f).Err() }()

	if f.externalID, err = changelist.GobID(f.host, f.change); err != nil {
		return err
	}
	if f.g, err = u.gFactory.MakeClient(ctx, f.host, f.luciProject); err != nil {
		return err
	}

	return f.update(ctx, common.CLID(r.GetClidHint()))
}

// ScheduleBatch schedules refresh of several Gerrit CLs.
//
// If called in a transaction, enqueues exactly one TQ task transactionally.
// This allows to write 1 Datastore entity during a transaction instead of N
// entities if Schedule() was used for each CL.
//
// Otherwise, enqueues 1 TQ task per CL non-transactionally.
func (u *Updater) ScheduleBatch(ctx context.Context, luciProject string, cls []*changelist.CL) error {
	tasks := make([]*RefreshGerritCL, len(cls))
	for i, cl := range cls {
		host, change, err := cl.ExternalID.ParseGobID()
		if err != nil {
			return errors.Annotate(err, "CL %d %q is not a Gerrit CL", cl.ID, cl.ExternalID).Err()
		}
		tasks[i] = &RefreshGerritCL{
			Host:        host,
			Change:      change,
			ClidHint:    int64(cl.ID),
			LuciProject: luciProject,
		}
	}
	switch {
	case len(tasks) == 1:
		// Optimization for the most frequent use-case of single-CL Runs.
		return u.Schedule(ctx, tasks[0])
	case datastore.CurrentTransaction(ctx) == nil:
		return u.RefreshBatch(ctx, &BatchRefreshGerritCL{Tasks: tasks})
	default:
		return u.tqd.AddTask(ctx, &tq.Task{
			Payload: &BatchRefreshGerritCL{Tasks: tasks},
			Title:   fmt.Sprintf("batch-%s-%d-cls", luciProject, len(tasks)),
		})
	}
}

// RefreshBatch schedules a refresh task per CL in a batch.
func (u *Updater) RefreshBatch(ctx context.Context, batch *BatchRefreshGerritCL) error {
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
		return errors.Annotate(err, "failed to schedule %d out of %d CLs refresh, keeping the most severe error", failed, total).Err()
	}
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
