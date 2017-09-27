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

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/identity"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
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
	// Create new Invocation entity, allocating its ID. It may be lost forever if
	// the current Job transaction fails. This is fine, it's no big deal since its
	// not discoverable by any queries.
	//
	// TODO(vadimsh): Delete it (best effort) on errors.
	inv, err := ctl.allocateInvocation(c, job, auth.CurrentIdentity(c), nil)
	if err != nil {
		return nil, err
	}

	// Make the job know that there's an invocation pending. This will make the
	// invocation show up in UI and API after the current transaction lands. If
	// it doesn't land, the new invocation will remain hanging as garbage, not
	// referenced by anything.
	job.ActiveInvocations = append(job.ActiveInvocations, inv.ID)

	// Kick off the launch sequence.
	if err := ctl.eng.kickLaunchInvocationsBatchTask(c, job.JobID, []int64{inv.ID}); err != nil {
		return nil, err
	}

	return resolvedFutureInvocation{invID: inv.ID}, nil
}

func (ctl *jobControllerV2) onInvUpdating(c context.Context, old, fresh *Invocation, timers []invocationTimer, triggers []task.Trigger) error {
	assertInTransaction(c)

	tasks := []*tq.Task{}

	// TODO(vadimsh): Implement timers.
	// TODO(vadimsh): Implement triggers.

	if !old.Status.Final() && fresh.Status.Final() {
		// When invocation finishes, make it appear in the list of finished
		// invocations (by setting the indexed field), and notify the parent job
		// about the completion, so it can kick off a new one or otherwise react.
		// Note that we can't open Job transaction here and have to use a task queue
		// task.
		fresh.IndexedJobID = fresh.JobID
		tasks = append(tasks, &tq.Task{
			Payload: &internal.InvocationFinishedTask{
				JobID: fresh.JobID,
				InvID: fresh.ID,
				// TODO(vadimsh): Add triggers here.
			},
		})
	} else if len(triggers) != 0 {
		// TODO(vadimsh): Emit <add a bunch of triggers> task.
	}

	if len(timers) != 0 {
		// TODO(vadimsh): Emit invocation timers.
	}

	return ctl.eng.cfg.Dispatcher.AddTask(c, tasks...)
}

////////////////////////////////////////////////////////////////////////////////

// allocateInvocation create new Invocation entity in a separate transaction.
func (ctl *jobControllerV2) allocateInvocation(c context.Context, job *Job, triggeredBy identity.Identity, triggers []task.Trigger) (*Invocation, error) {
	var inv *Invocation
	err := runIsolatedTxn(c, func(c context.Context) (err error) {
		inv, err = ctl.eng.newInvocation(c, job.JobID, &Invocation{
			Started:          clock.Now(c).UTC(),
			TriggeredBy:      triggeredBy,
			IncomingTriggers: triggers,
			Revision:         job.Revision,
			RevisionURL:      job.RevisionURL,
			Task:             job.Task,
			TriggeredJobIDs:  job.TriggeredJobIDs,
			Status:           task.StatusStarting,
		})
		if err != nil {
			return
		}
		inv.debugLog(c, "New invocation initialized")
		if triggeredBy != "" {
			inv.debugLog(c, "Manually triggered by %s", triggeredBy)
		}
		return transient.Tag.Apply(datastore.Put(c, inv))
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
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
