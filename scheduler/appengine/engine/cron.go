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
	"time"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"

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
		return errors.Annotate(err, "bad schedule %q", job.EffectiveSchedule()).Err()
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
		return errors.Annotate(err, "callback error").Err()
	}

	tasks := []*tq.Task{}
	for _, action := range machine.Actions {
		switch a := action.(type) {
		case cron.TickLaterAction:
			logging.Infof(c, "Scheduling tick %d after %s", a.TickNonce, a.When.Sub(now))
			tasks = append(tasks, &tq.Task{
				Payload: &internal.CronTickTask{
					JobId:     job.JobID,
					TickNonce: a.TickNonce,
				},
				ETA: a.When,
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
			return errors.Reason("unknown action %T emitted by the cron machine", action).Err()
		}
	}
	if err := disp.AddTask(c, tasks...); err != nil {
		return errors.Annotate(err, "failed to enqueue emitted actions").Err()
	}

	job.Cron = machine.State
	return nil
}

// cronTrigger generates a trigger struct from an invocation request generated
// by the cron state machine.
func cronTrigger(a cron.StartInvocationAction, now time.Time) *internal.Trigger {
	return &internal.Trigger{
		Id:      fmt.Sprintf("cron:v1:%d", a.Generation),
		Created: google.NewTimestamp(now),
		Payload: &internal.Trigger_Cron{
			Cron: &api.CronTrigger{
				Generation: a.Generation,
			},
		},
	}
}
