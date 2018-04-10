// Copyright 2017 The LUCI Authors.
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
	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/scheduler/appengine/engine/cron"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// jobControllerV2 implements jobController using v2 data structures.
type jobControllerV2 struct {
	eng *engineImpl
}

func (ctl *jobControllerV2) onJobScheduleChange(c context.Context, job *Job) error {
	return pokeCron(c, job, ctl.eng.cfg.Dispatcher, func(m *cron.Machine) error {
		// TODO(vadimsh): Remove this branch once all jobs are updated to v2. For
		// now it allows to manually kick start v2 cron by pausing and then
		// unpausing a job.
		if job.Enabled {
			m.Enable()
		}
		m.OnScheduleChange()
		return nil
	})
}

func (ctl *jobControllerV2) onJobEnabled(c context.Context, job *Job) error {
	return pokeCron(c, job, ctl.eng.cfg.Dispatcher, func(m *cron.Machine) error {
		m.Enable()
		return nil
	})
}

func (ctl *jobControllerV2) onJobDisabled(c context.Context, job *Job) error {
	return pokeCron(c, job, ctl.eng.cfg.Dispatcher, func(m *cron.Machine) error {
		m.Disable()
		return nil
	})
}

func (ctl *jobControllerV2) onJobCronTick(c context.Context, job *Job, tick *internal.CronTickTask) error {
	return pokeCron(c, job, ctl.eng.cfg.Dispatcher, func(m *cron.Machine) error {
		// OnTimerTick returns an error if the tick happened too soon. Mark this
		// error as transient to trigger task queue retry at a later time.
		return transient.Tag.Apply(m.OnTimerTick(tick.TickNonce))
	})
}

func (ctl *jobControllerV2) onJobAbort(c context.Context, job *Job) (invs []int64, err error) {
	return nil, nil
}

func (ctl *jobControllerV2) onJobForceInvocation(c context.Context, job *Job) (FutureInvocation, error) {
	invs, err := ctl.eng.enqueueInvocations(c, job, []task.Request{
		{TriggeredBy: auth.CurrentIdentity(c)},
	})
	if err != nil {
		return nil, err
	}
	return resolvedFutureInvocation{invID: invs[0].ID}, nil
}

func (ctl *jobControllerV2) onInvUpdating(c context.Context, old, fresh *Invocation, timers []*internal.Timer, triggers []*internal.Trigger) error {
	assertInTransaction(c)

	if fresh.Status.Final() && len(timers) > 0 {
		panic("finished invocations must not emit timer, ensured by taskController")
	}

	// Register new timers in the Invocation entity. Used to reject duplicate
	// task queue calls: only tasks that reference a timer in the pending timers
	// set are accepted.
	if len(timers) != 0 {
		err := mutateTimersList(&fresh.PendingTimersRaw, func(out *[]*internal.Timer) {
			*out = append(*out, timers...)
		})
		if err != nil {
			return err
		}
	}

	// Register emitted triggers in the Invocation entity. Used mostly for UI.
	if len(triggers) != 0 {
		err := mutateTriggersList(&fresh.OutgoingTriggersRaw, func(out *[]*internal.Trigger) {
			*out = append(*out, triggers...)
		})
		if err != nil {
			return err
		}
	}

	// Prepare FanOutTriggersTask if we are emitting triggers for real. Skip this
	// if no job is going to get them.
	var fanOutTriggersTask *internal.FanOutTriggersTask
	if len(triggers) != 0 && len(fresh.TriggeredJobIDs) != 0 {
		fanOutTriggersTask = &internal.FanOutTriggersTask{
			JobIds:   fresh.TriggeredJobIDs,
			Triggers: triggers,
		}
	}

	var tasks []*tq.Task

	if !old.Status.Final() && fresh.Status.Final() {
		// When invocation finishes, make it appear in the list of finished
		// invocations (by setting the indexed field), and notify the parent job
		// about the completion, so it can kick off a new one or otherwise react.
		// Note that we can't open Job transaction here and have to use a task queue
		// task. Bundle fanOutTriggersTask with this task, since we can. No need to
		// create two separate tasks.
		fresh.IndexedJobID = fresh.JobID
		tasks = append(tasks, &tq.Task{
			Payload: &internal.InvocationFinishedTask{
				JobId:    fresh.JobID,
				InvId:    fresh.ID,
				Triggers: fanOutTriggersTask,
			},
		})
	} else if fanOutTriggersTask != nil {
		tasks = append(tasks, &tq.Task{Payload: fanOutTriggersTask})
	}

	// When emitting more than 1 timer (this is rare) use an intermediary task,
	// to avoid getting close to limit of number of tasks in a transaction. When
	// emitting 1 timer (most common case), don't bother, since we aren't winning
	// anything.
	switch {
	case len(timers) == 1:
		tasks = append(tasks, &tq.Task{
			ETA: google.TimeFromProto(timers[0].Eta),
			Payload: &internal.TimerTask{
				JobId: fresh.JobID,
				InvId: fresh.ID,
				Timer: timers[0],
			},
		})
	case len(timers) > 1:
		tasks = append(tasks, &tq.Task{
			Payload: &internal.ScheduleTimersTask{
				JobId:  fresh.JobID,
				InvId:  fresh.ID,
				Timers: timers,
			},
		})
	}

	return ctl.eng.cfg.Dispatcher.AddTask(c, tasks...)
}

////////////////////////////////////////////////////////////////////////////////

// resolvedFutureInvocation implements FutureInvocation by returning known
// invocation ID.
type resolvedFutureInvocation struct {
	invID int64
}

func (r resolvedFutureInvocation) InvocationID(context.Context) (int64, error) {
	return r.invID, nil
}
