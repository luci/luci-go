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
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/scheduler/appengine/engine/dsset"
)

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
	jobID string // ID of the job, must be provided externally

	// The rest of fields are used internally.

	finishedSet  *invocationIDSet
	finishedList *dsset.Listing

	garbage dsset.Garbage // collected inside the transaction, cleaned outside
}

// prepare fetches all pending triggers and events from dsset structs.
func (op *triageOp) prepare(c context.Context) error {
	op.finishedSet = recentlyFinishedSet(c, op.jobID)

	// Grab a list of recently finished invocations to remove them from
	// ActiveInvocations list.
	var err error
	op.finishedList, err = op.finishedSet.List(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to grab a set of recently finished invocations")
		return err
	}
	logging.Infof(c, "Recently finished set: %d items, %d garbage", len(op.finishedList.Items), len(op.finishedList.Garbage))

	// Remove old tombstones to keep the set tidy. We fail hard here on errors to
	// make sure progress stops if garbage can't be cleaned up for some reason.
	// It is better to fail early rather than silently accumulate tons of garbage
	// until everything grinds to a halt.
	if err := dsset.CleanupGarbage(c, op.finishedList.Garbage); err != nil {
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
	// Reset state collecting in the transaction in case this is a retry.
	op.garbage = nil

	// Tidy ActiveInvocations list by removing all recently finished invocations.
	tidyOp, err := op.tidyActiveInvocations(c, job)
	if err != nil {
		return err
	}

	// Submit set modifications. This may produce more garbage that we need to
	// cleanup later (outside the transaction).
	popOps := []*dsset.PopOp{}
	if tidyOp != nil {
		popOps = append(popOps, tidyOp)
	}
	if op.garbage, err = dsset.FinishPop(c, popOps...); err != nil {
		return err
	}

	logging.Infof(c, "Active invocations: %v", job.ActiveInvocations)
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
