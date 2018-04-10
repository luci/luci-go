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
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

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

	// triggeringPolicy decides how to convert a set of pending triggers into
	// a bunch of new invocations.
	//
	// TODO(vadimsh): Convert to interface. Document the contract.
	triggeringPolicy func(c context.Context, job *Job, triggers []*internal.Trigger) ([]task.Request, error)

	// enqueueInvocations transactionally creates and starts new invocations.
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

	// Tidy ActiveInvocations list by removing all recently finished invocations.
	tidyOp, err := op.tidyActiveInvocations(c, job)
	if err != nil {
		return err
	}
	logging.Infof(c, "Active invocations: %v", job.ActiveInvocations)

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

// tidyActiveInvocations removes finished invocations from ActiveInvocations.
//
// Called within a transaction. Mutates job. Returns an open dsset.PopOp that
// must be eventually submitted with dsset.FinishPop.
func (op *triageOp) tidyActiveInvocations(c context.Context, job *Job) (*dsset.PopOp, error) {
	// Note that per dsset API we need to do BeginPop if there's some garbage,
	// even if Items is empty. We can skip this if both lists are empty though.
	if len(op.finishedList.Items) == 0 && len(op.finishedList.Garbage) == 0 {
		return nil, nil
	}

	popOp, err := op.finishedSet.BeginPop(c, op.finishedList)
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
	// preserving the order.
	if len(reallyFinished) != 0 {
		filtered := make([]int64, 0, len(job.ActiveInvocations))
		for _, id := range job.ActiveInvocations {
			if _, yep := reallyFinished[id]; yep {
				logging.Infof(c, "Invocation %d is acknowledged as finished", id)
			} else {
				filtered = append(filtered, id) // still running
			}
		}
		job.ActiveInvocations = filtered
	}

	return popOp, nil
}

// processTriggers pops pending triggers, converting them into invocations.
func (op *triageOp) processTriggers(c context.Context, job *Job) (*dsset.PopOp, error) {
	// Note that per dsset API we need to do BeginPop if there's some garbage,
	// even if Items is empty. We can skip this if both lists are empty though.
	if len(op.readyTriggers) == 0 && len(op.triggersList.Garbage) == 0 {
		return nil, nil
	}

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

	if len(triggers) == 0 {
		return popOp, nil
	}

	// Look at what's left and decide what invocations to start (if any) by
	// getting a bunch of task.Requests from the triggering policy function.
	now := clock.Now(c)
	logging.Infof(c, "Pending triggers:")
	for _, t := range triggers {
		logging.Infof(c, "  %q submitted %s ago by %q inv %d",
			t.Id, now.Sub(google.TimeFromProto(t.Created)), t.JobId, t.InvocationId)
	}
	requests, err := op.triggeringPolicy(c, job, triggers)
	if err != nil {
		return nil, err
	}

	// Actually pop all consumed triggers and start the corresponding invocations.
	// Note that this modifies job.ActiveInvocations list.
	if len(requests) != 0 {
		for _, r := range requests {
			for _, t := range r.IncomingTriggers {
				popOp.Pop(t.Id)
			}
		}
		if err := op.enqueueInvocations(c, job, requests); err != nil {
			return nil, err
		}
	}

	return popOp, nil
}
