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
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/scheduler/appengine/internal"
)

// jobControllerV2 implements jobController using v2 data structures.
type jobControllerV2 struct {
	eng *engineImpl
}

func (ctl *jobControllerV2) onJobScheduleChange(c context.Context, job *Job) error {
	return nil
}

func (ctl *jobControllerV2) onJobEnabled(c context.Context, job *Job) error {
	return nil
}

func (ctl *jobControllerV2) onJobDisabled(c context.Context, job *Job) error {
	return nil
}

func (ctl *jobControllerV2) onJobAbort(c context.Context, job *Job) (invs []int64, err error) {
	return nil, nil
}

func (ctl *jobControllerV2) onJobForceInvocation(c context.Context, job *Job) (FutureInvocation, error) {
	invs, err := ctl.eng.enqueueInvocations(c, job, []InvocationRequest{
		{TriggeredBy: auth.CurrentIdentity(c)},
	})
	if err != nil {
		return nil, err
	}
	return resolvedFutureInvocation{invID: invs[0].ID}, nil
}

func (ctl *jobControllerV2) onInvUpdating(c context.Context, old, fresh *Invocation, timers []invocationTimer, triggers []*internal.Trigger) error {
	assertInTransaction(c)

	// TODO(vadimsh): Implement timers.

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

	if len(timers) != 0 {
		// TODO(vadimsh): Emit invocation timers.
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
