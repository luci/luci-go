// Copyright 2018 The LUCI Authors.
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

package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/info"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/engine/cron"
	"go.chromium.org/luci/scheduler/appengine/internal"
)

// pokeCron instantiates a cron state machine and calls the callback to advance
// its state.
//
// Should be part of a Job transaction. If the callback succeeds, job.State
// is updated with the new state and all emitted actions are dispatched to the
// task queue. The job entity should eventually be saved as part of this
// transaction.
//
// Returns fatal errors if something is not right with the job definition or
// state. Returns transient errors on task queue failures.
func pokeCron(c context.Context, job *Job, disp *tq.Dispatcher, cb func(m *cron.Machine) error) error {
	assertInTransaction(c)

	sched, err := job.ParseSchedule()
	if err != nil {
		return errors.Fmt("bad schedule %q: %w", job.EffectiveSchedule(), err)
	}

	now := clock.Now(c).UTC()
	rnd := mathrand.Get(c)

	machine := &cron.Machine{
		Now:      now,
		Schedule: sched,
		Nonce:    func() int64 { return rnd.Int63() + 1 },
		State:    job.Cron,
	}
	if err := cb(machine); err != nil {
		return errors.Fmt("callback error: %w", err)
	}

	tasks := []*tq.Task{}
	for _, action := range machine.Actions {
		switch a := action.(type) {
		case cron.TickLaterAction:
			delay := a.When.Sub(now)
			// Some very infrequent schedules may have next tick weeks in the future.
			// However, Task Queue limits tasks to at most 30 days in the future.
			// Thus, waiting may be done via a chain of several TQ tasks. To allow
			// cron.Machine.OnTimerTick to distinguish such chains from accidentally
			// too early execution of a specific TQ task, ensure intermediate TQ tasks
			// are at least 1 hour before actual tick.
			maxDelay := 15 * 24 * time.Hour // conservative 15 days.
			fudge := 1 * time.Hour
			if strings.HasSuffix(info.AppID(c), "-dev") {
				// Use lower numbers on -dev to exercise this codepath frequently.
				maxDelay = 2 * time.Minute
				fudge = 1 * time.Minute
			}
			if delay > maxDelay+fudge {
				logging.Infof(c, "Scheduling intermediary tick %d after %s instead of intended %s", a.TickNonce, maxDelay, delay)
				delay = maxDelay
			} else {
				logging.Infof(c, "Scheduling tick %d after %s", a.TickNonce, delay)
			}
			tasks = append(tasks, &tq.Task{
				Payload: &internal.CronTickTask{
					JobId:     job.JobID,
					TickNonce: a.TickNonce,
				},
				Delay: delay,
			})
		case cron.StartInvocationAction:
			trigger := cronTrigger(a, now)
			logging.Infof(c, "Emitting cron trigger %s", trigger.Id)
			tasks = append(tasks, &tq.Task{
				Payload: &internal.EnqueueTriggersTask{
					JobId:    job.JobID,
					Triggers: []*internal.Trigger{trigger},
				},
			})
		default:
			return errors.Fmt("unknown action %T emitted by the cron machine", action)
		}
	}
	if err := disp.AddTask(c, tasks...); err != nil {
		return errors.Fmt("failed to enqueue emitted actions: %w", err)
	}

	job.Cron = machine.State
	return nil
}

// cronTrigger generates a trigger struct from an invocation request generated
// by the cron state machine.
func cronTrigger(a cron.StartInvocationAction, now time.Time) *internal.Trigger {
	return &internal.Trigger{
		Id:      fmt.Sprintf("cron:v1:%d", a.Generation),
		Created: timestamppb.New(now),
		Payload: &internal.Trigger_Cron{
			Cron: &api.CronTrigger{
				Generation: a.Generation,
			},
		},
	}
}
