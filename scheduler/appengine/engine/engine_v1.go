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
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/scheduler/appengine/task"
)

// jobControllerV1 implements jobController using v1 data structures.
type jobControllerV1 struct {
	eng *engineImpl
}

func (ctl *jobControllerV1) onJobScheduleChange(c context.Context, job *Job) error {
	return ctl.eng.rollSM(c, job, func(sm *StateMachine) error {
		sm.OnScheduleChange()
		return nil
	})
}

func (ctl *jobControllerV1) onJobEnabled(c context.Context, job *Job) error {
	return ctl.eng.rollSM(c, job, func(sm *StateMachine) error {
		sm.OnJobEnabled()
		return nil
	})
}

func (ctl *jobControllerV1) onJobDisabled(c context.Context, job *Job) error {
	return ctl.eng.rollSM(c, job, func(sm *StateMachine) error {
		sm.OnJobDisabled()
		return nil
	})
}

func (ctl *jobControllerV1) onJobAbort(c context.Context, job *Job) (invs []int64, err error) {
	err = ctl.eng.rollSM(c, job, func(sm *StateMachine) error {
		if sm.State.InvocationID != 0 {
			invs = append(invs, sm.State.InvocationID)
		}
		sm.OnManualAbort()
		return nil
	})
	return
}

func (ctl *jobControllerV1) onJobForceInvocation(c context.Context, job *Job) (FutureInvocation, error) {
	var nonce int64
	err := ctl.eng.rollSM(c, job, func(sm *StateMachine) error {
		if err := sm.OnManualInvocation(auth.CurrentIdentity(c)); err != nil {
			return err
		}
		nonce = sm.State.InvocationNonce
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &nonceFutureInvocation{jobID: job.JobID, nonce: nonce}, nil
}

func (ctl *jobControllerV1) onInvUpdating(c context.Context, old, fresh *Invocation, timers []invocationTimer, triggers []task.Trigger) error {
	assertInTransaction(c)

	jobID := fresh.jobID()
	invID := fresh.ID
	eng := ctl.eng

	if len(timers) > 0 {
		if err := eng.enqueueInvTimers(c, jobID, invID, timers); err != nil {
			return err
		}
	}

	if len(triggers) > 0 {
		if err := eng.enqueueTriggers(c, fresh.TriggeredJobIDs, triggers); err != nil {
			return err
		}
		fresh.OutgoingTriggers = append(fresh.OutgoingTriggers, triggers...)
	}

	hasStartedOrFailed := old.Status.Initial() && !fresh.Status.Initial()
	hasFinished := !old.Status.Final() && fresh.Status.Final()
	if !hasStartedOrFailed && !hasFinished {
		return nil // nothing that could affect the Job happened
	}

	// Fetch the up-to-date state of the job. Not a fatal error if not there.
	job := Job{JobID: jobID}
	switch err := datastore.Get(c, &job); {
	case err == datastore.ErrNoSuchEntity:
		logging.Errorf(c, "Active job is unexpectedly gone")
		return nil
	case err != nil:
		return transient.Tag.Apply(err)
	}

	// Still have this invocation associate with the job? Not big deal if not.
	if job.State.InvocationID != invID {
		logging.Warningf(c, "The invocation is no longer current, the current is %d", job.State.InvocationID)
		return nil
	}

	// Make the state machine transitions, mutating a copy of 'job'.
	modified := job
	if hasStartedOrFailed {
		err := eng.rollSM(c, &modified, func(sm *StateMachine) error {
			sm.OnInvocationStarted(invID)
			return nil
		})
		if err != nil {
			return err
		}
	}
	if hasFinished {
		err := eng.rollSM(c, &modified, func(sm *StateMachine) error {
			sm.OnInvocationDone(invID)
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Don't bother doing an RPC if nothing has changed.
	if !modified.IsEqual(&job) {
		return transient.Tag.Apply(datastore.Put(c, &modified))
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

// nonceFutureInvocation implements FutureInvocation by polling invocations
// based on nonce.
type nonceFutureInvocation struct {
	jobID string
	nonce int64
}

// InvocationID returns an ID of the invocation or 0 if not started yet.
func (f *nonceFutureInvocation) InvocationID(c context.Context) (int64, error) {
	q := datastore.NewQuery("Invocation").Eq("InvocationNonce", f.nonce)
	entities := []*Invocation{}
	if err := datastore.GetAll(c, q, &entities); err != nil {
		return 0, transient.Tag.Apply(err)
	}

	// Keep only ones that match the job, pick the most recent one. Filtering in
	// the code here is cheaper than adding a composite datastore index. Almost
	// always 'entities' is small.
	bestTS := time.Time{}
	invID := int64(0)
	for _, inv := range entities {
		// Note: this code is supposed to work only with v1 entities that have
		// JobKey set.
		if inv.JobKey.StringID() == f.jobID {
			if inv.Started.After(bestTS) {
				invID = inv.ID
				bestTS = inv.Started
			}
		}
	}

	return invID, nil
}
