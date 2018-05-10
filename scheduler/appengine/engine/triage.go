// Copyright 2015 The LUCI Authors.
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
	"sync"

	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/scheduler/appengine/engine/cron"
	"go.chromium.org/luci/scheduler/appengine/engine/dsset"
	"go.chromium.org/luci/scheduler/appengine/engine/policy"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// TODO(vadimsh): Surface triage log and status in UI and Monitoring.

// errTriagePrepareFail is returned by 'prepare' on errors.
var errTriagePrepareFail = errors.New("error while fetching sets, see logs", transient.Tag)

// triageOp is a short lived struct that represents in-flight triage operation.
//
// A triage is a process of looking at what pending triggers/events a job has
// accumulated and deciding what to do with them (e.g. starting new invocations
// or waiting).
//
// There are 3 stages of the triage operation:
//   * Pre-transaction to gather pending events.
//   * Transaction to "atomically" consume the events.
//   * Post-transaction to cleanup garbage, update monitoring, etc.
type triageOp struct {
	// jobID is ID of the job being examined, must be provided externally.
	jobID string

	// dispatcher routes task queue tasks.
	dispatcher *tq.Dispatcher

	// policyFactory knows how to instantiate triggering policies.
	//
	// Usually it is policy.New, but may be replaced in tests.
	policyFactory func(*messages.TriggeringPolicy) (policy.Func, error)

	// enqueueInvocations transactionally creates and starts new invocations.
	//
	// Implemented by the engine, see engineImpl.enqueueInvocations.
	enqueueInvocations func(c context.Context, job *Job, req []task.Request) error

	// The rest of fields are used internally.

	finishedSet  *invocationIDSet
	finishedList *dsset.Listing

	triggersSet   *triggersSet
	triggersList  *dsset.Listing
	readyTriggers []*internal.Trigger // same as triggersList, but deserialized and sorted

	garbage dsset.Garbage // collected inside the transaction, cleaned outside
}

// prepare fetches all pending triggers and events from dsset structs.
func (op *triageOp) prepare(c context.Context) error {
	op.finishedSet = recentlyFinishedSet(c, op.jobID)
	op.triggersSet = pendingTriggersSet(c, op.jobID)

	wg := sync.WaitGroup{}
	wg.Add(2)

	// Grab all pending triggers to decided what to do with them.
	var triggersErr error
	go func() {
		defer wg.Done()
		op.triggersList, op.readyTriggers, triggersErr = op.triggersSet.Triggers(c)
		if triggersErr != nil {
			logging.WithError(triggersErr).Errorf(c, "Failed to grab a set of pending triggers")
		} else {
			sortTriggers(op.readyTriggers)
		}
	}()

	// Grab a list of recently finished invocations to remove them from
	// ActiveInvocations list.
	var finishedErr error
	go func() {
		defer wg.Done()
		op.finishedList, finishedErr = op.finishedSet.List(c)
		if finishedErr != nil {
			logging.WithError(finishedErr).Errorf(c, "Failed to grab a set of recently finished invocations")
			return
		}
	}()

	wg.Wait()
	if triggersErr != nil || finishedErr != nil {
		return errTriagePrepareFail // the original error is already logged
	}

	logging.Infof(c, "Pending triggers set:  %d items, %d garbage", len(op.triggersList.Items), len(op.triggersList.Garbage))
	logging.Infof(c, "Recently finished set: %d items, %d garbage", len(op.finishedList.Items), len(op.finishedList.Garbage))

	// Remove old tombstones to keep the set tidy. We fail hard here on errors to
	// make sure progress stops if garbage can't be cleaned up for some reason.
	// It is better to fail early rather than silently accumulate tons of garbage
	// until everything grinds to a halt.
	if err := dsset.CleanupGarbage(c, op.triggersList.Garbage, op.finishedList.Garbage); err != nil {
		logging.WithError(err).Errorf(c, "Failed to cleanup dsset garbage")
		return err
	}

	return nil
}

// transaction must be called within a job transaction.
//
// It pops pending triggers/events, producing invocations or other events along
// the way.
//
// This method may be called multiple times (when the transaction is retried).
func (op *triageOp) transaction(c context.Context, job *Job) error {
	// Reset state collected in the transaction in case this is a retry.
	op.garbage = nil

	// Tidy ActiveInvocations list by moving all recently finished invocations to
	// FinishedInvocations list.
	tidyOp, err := op.tidyActiveInvocations(c, job)
	if err != nil {
		return err
	}

	// Process pending triggers set by emitting new invocations. Note that this
	// modifies ActiveInvocations list when emitting invocations.
	triggersOp, err := op.processTriggers(c, job)
	if err != nil {
		return err
	}

	// If nothing is running anymore, make sure the cron is ticking again. This is
	// useful for schedules like "with 10min interval" that initiate an invocation
	// after some time after the previous one finishes. This call submits at most
	// one task to TQ. Note that there's no harm in calling this multiple times,
	// only the first call will actually do something.
	if len(job.ActiveInvocations) == 0 {
		err := pokeCron(c, job, op.dispatcher, func(m *cron.Machine) error {
			m.RewindIfNecessary()
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Submit set modifications. This may produce more garbage that we need to
	// cleanup later (outside the transaction).
	popOps := []*dsset.PopOp{}
	if tidyOp != nil {
		popOps = append(popOps, tidyOp)
	}
	if triggersOp != nil {
		popOps = append(popOps, triggersOp)
	}
	if op.garbage, err = dsset.FinishPop(c, popOps...); err != nil {
		return err
	}

	return nil
}

// finalize is called after successfully submitted transaction to delete any
// produced garbage, update monitoring counters, etc.
//
// It is best effort. We can't do anything meaningful if it fails: the
// transaction has already landed, there's no way to unland it.
func (op *triageOp) finalize(c context.Context) {
	if len(op.garbage) != 0 {
		logging.Infof(c, "Cleaning up storage of %d dsset items", len(op.garbage))
		if err := dsset.CleanupGarbage(c, op.garbage); err != nil {
			logging.WithError(err).Warningf(c, "Best effort cleanup failed")
		}
	}
}

////////////////////////////////////////////////////////////////////////////////

// tidyActiveInvocations removes finished invocations from ActiveInvocations,
// and adds them to FinishedInvocations.
//
// Called within a transaction. Mutates job. Returns an open dsset.PopOp that
// must be eventually submitted with dsset.FinishPop.
func (op *triageOp) tidyActiveInvocations(c context.Context, job *Job) (*dsset.PopOp, error) {
	now := clock.Now(c).UTC()

	// Deserialize the list of recently finished invocations, as stored in the
	// entity. Discard old items right away. If it is broken, log, but proceed.
	// It is ~OK to loose it (this will temporary cause some API calls to return
	// incomplete data).
	finishedRecord, err := filteredFinishedInvs(
		job.FinishedInvocationsRaw, now.Add(-FinishedInvocationsHorizon))
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to unmarshal FinishedInvocationsRaw, skipping")
	}

	// Note that per dsset API we need to do BeginPop if there's some garbage,
	// even if Items is empty. We can skip this if both lists are empty though.
	var popOp *dsset.PopOp
	if len(op.finishedList.Items) != 0 || len(op.finishedList.Garbage) != 0 {
		popOp, err = op.finishedSet.BeginPop(c, op.finishedList)
		if err != nil {
			return nil, err
		}

		// Items can have IDs popped by some other transaction already. Collect
		// only ones consumed by us here.
		reallyFinished := make(map[int64]struct{}, len(op.finishedList.Items))
		for _, itm := range op.finishedList.Items {
			if popOp.Pop(itm.ID) {
				reallyFinished[op.finishedSet.ItemToInvID(&itm)] = struct{}{}
			}
		}

		// Remove IDs of all finished invocations from ActiveInvocations list,
		// preserving the order, and add them to the finished invocations list.
		if len(reallyFinished) != 0 {
			filtered := make([]int64, 0, len(job.ActiveInvocations))
			for _, id := range job.ActiveInvocations {
				if _, yep := reallyFinished[id]; !yep {
					filtered = append(filtered, id) // still running
					continue
				}
				logging.Infof(c, "Invocation %d is acknowledged as finished", id)
				finishedRecord = append(finishedRecord, &internal.FinishedInvocation{
					InvocationId: id,
					Finished:     google.NewTimestamp(now),
				})
			}
			job.ActiveInvocations = filtered
		}
	}

	// Marshal back FinishedInvocationsRaw after it has been updated.
	job.FinishedInvocationsRaw = marshalFinishedInvs(finishedRecord)

	// Nice looking logging.
	invIDs := make([]int64, len(finishedRecord))
	for i := range finishedRecord {
		invIDs[i] = finishedRecord[i].InvocationId
	}
	logging.Infof(c, "Active invocations: %v", job.ActiveInvocations)
	logging.Infof(c, "Recently finished:  %v", invIDs)

	return popOp, nil
}

// processTriggers pops pending triggers, converting them into invocations.
func (op *triageOp) processTriggers(c context.Context, job *Job) (*dsset.PopOp, error) {
	popOp, err := op.triggersSet.BeginPop(c, op.triggersList)
	if err != nil {
		return nil, err
	}

	// Filter out all triggers already popped by some other transaction.
	triggers := make([]*internal.Trigger, 0, len(op.readyTriggers))
	for _, t := range op.readyTriggers {
		if popOp.CanPop(t.Id) {
			triggers = append(triggers, t)
		}
	}

	// Look at what's left and decide what invocations to start (if any).
	if len(triggers) != 0 {
		now := clock.Now(c)
		logging.Infof(c, "Pending triggers:")
		for _, t := range triggers {
			logging.Infof(c, "  %q submitted %s ago by %q inv %d",
				t.Id, now.Sub(google.TimeFromProto(t.Created)), t.JobId, t.InvocationId)
		}

		// If the job is paused or disabled, just pop all triggers without starting
		// anything. Note: there's a best-effort shortcut for ignoring triggers
		// for paused jobs in execEnqueueTriggersTask.
		if job.Paused || !job.Enabled {
			logging.Infof(c, "The job is inactive, clearing the pending triggers queue")
			for _, t := range triggers {
				popOp.Pop(t.Id)
			}
			return popOp, nil
		}
	}

	// Otherwise ask the policy to convert triggers into invocations. Note that
	// triggers is not the only input to the triggering policy, so we call it
	// on each triage, even if there's no pending triggers.
	out := op.triggeringPolicy(c, job, triggers)

	// Actually pop all consumed triggers and start the corresponding invocations.
	// Note that this modifies job.ActiveInvocations list.
	if len(out.Requests) != 0 {
		for _, r := range out.Requests {
			for _, t := range r.IncomingTriggers {
				popOp.Pop(t.Id)
			}
		}
		if err := op.enqueueInvocations(c, job, out.Requests); err != nil {
			return nil, err
		}
	}

	return popOp, nil
}

// triggeringPolicy decides how to convert a set of pending triggers into
// a bunch of new invocations.
//
// Called within a job transaction. Must not do any expensive calls.
func (op *triageOp) triggeringPolicy(c context.Context, job *Job, triggers []*internal.Trigger) policy.Out {
	var policyFunc policy.Func
	p, err := policy.UnmarshalDefinition(job.TriggeringPolicyRaw)
	if err == nil {
		policyFunc, err = op.policyFactory(p)
	}
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to instantiate the triggering policy function, using the default policy instead")
		policyFunc = policy.Default()
	}
	return policyFunc(policyFuncEnv{c, op}, policy.In{
		Now:               clock.Now(c).UTC(),
		ActiveInvocations: job.ActiveInvocations,
		Triggers:          triggers,
	})
}

// policyFuncEnv implements policy.Environment through triageOp.
type policyFuncEnv struct {
	ctx context.Context
	op  *triageOp
}

func (e policyFuncEnv) DebugLog(format string, args ...interface{}) {
	// TODO(vadimsh): Log this into datastore too, so that we have access to
	// the triage log in the UI.
	logging.Infof(e.ctx, format, args...)
}
