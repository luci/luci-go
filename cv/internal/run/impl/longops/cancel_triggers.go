// Copyright 2021 The LUCI Authors.
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

package longops

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/cancel"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

// CancelTriggersOp cancels the triggers for the provided CLs.
//
// CancelTriggersOp keeps retrying on lease error and transient failure for each
// CL till the long op deadline is exceeded or cancellation either succeeds
// or fails non-transiently.
//
// CancelTriggersOp doesn't obey longop's cancellation request because if
// this long op is left half-done, for example, triggers on half of the CLs are
// untouched, a new Run may be created for those CLs.
//
// CancelTriggersOp is a single-use object.
type CancelTriggersOp struct {
	*Base
	GFactory  gerrit.Factory
	CLMutator *changelist.Mutator
	// CancelConcurrency is the number of CLs that will be cancelled concurrently.
	//
	// Default is 8.
	CancelConcurrency int

	// Private fields that will be populated internally during long op execution.

	inputs  []cancel.Input
	results []cancelResult

	// testAfterTryCancelFn is always called after each try to cancel the trigger
	// of a CL.
	//
	// This is only set for testing purpose.
	testAfterTryCancelFn func()
}

const defaultCancelConcurrency = 8

// Do actually cancels the triggers.
func (op *CancelTriggersOp) Do(ctx context.Context) (*eventpb.LongOpCompleted, error) {
	op.assertCalledOnce()

	if err := op.loadInputs(ctx); err != nil {
		return nil, err
	}

	op.executeInParallel(ctx)

	longOpStatus := eventpb.LongOpCompleted_SUCCEEDED // be optimistic
	ct := &eventpb.LongOpCompleted_CancelTriggers{
		Results: make([]*eventpb.LongOpCompleted_CancelTriggers_Result, len(op.results)),
	}
	var lastTransErr, lastPermErr error
	for i, result := range op.results {
		cl := op.inputs[i].CL
		ct.Results[i] = &eventpb.LongOpCompleted_CancelTriggers_Result{
			Id:         int64(cl.ID),
			ExternalId: string(cl.ExternalID),
		}
		switch err := result.err; {
		case err == nil:
			ct.Results[i].Detail = &eventpb.LongOpCompleted_CancelTriggers_Result_SuccessInfo{
				SuccessInfo: &eventpb.LongOpCompleted_CancelTriggers_Result_Success{
					CancelledAt: timestamppb.New(result.cancelledAt),
				},
			}
		default:
			longOpStatus = eventpb.LongOpCompleted_FAILED
			ct.Results[i].Detail = &eventpb.LongOpCompleted_CancelTriggers_Result_FailureInfo{
				FailureInfo: &eventpb.LongOpCompleted_CancelTriggers_Result_Failure{
					FailureMessage: err.Error(),
				},
			}
			logging.Errorf(ctx, "failed to cancel the trigger of CL %d %q: %s", cl.ID, cl.ExternalID, err)
			if transient.Tag.In(err) {
				lastTransErr = err
			} else {
				lastPermErr = err
			}
		}
	}
	ret := &eventpb.LongOpCompleted{
		Status: longOpStatus,
		Result: &eventpb.LongOpCompleted_CancelTriggers_{
			CancelTriggers: ct,
		},
	}
	switch ctxErr := ctx.Err(); {
	// Returns the event in error case as well because the event will be
	// reported back to Run Manager.
	case ctxErr == context.DeadlineExceeded:
		logging.Errorf(ctx, "running out of time to cancel triggers")
		return ret, ctxErr
	case ctxErr == context.Canceled:
		logging.Errorf(ctx, "context is cancelled while cancelling triggers")
		return ret, ctxErr
	case ctxErr != nil:
		panic(fmt.Errorf("unexpected context error: %s", ctxErr))
	case lastPermErr != nil:
		return ret, lastPermErr
	case lastTransErr != nil:
		// Don't return a transient error to prevent long op from retrying.
		// The transient error should have been retried many times in this long op.
		return ret, transient.Tag.Off().Apply(lastTransErr)
	default:
		return ret, nil
	}
}

func (op *CancelTriggersOp) loadInputs(ctx context.Context) error {
	var (
		clsToCancel         []*changelist.CL
		triggers            map[common.CLID]*run.Triggers
		cfg                 *prjcfg.ConfigGroup
		allRunCLExternalIDs []changelist.ExternalID
	)
	eg, ctx := errgroup.WithContext(ctx)
	requests := op.Op.GetCancelTriggers().GetRequests()
	eg.Go(func() (err error) {
		clids := make(common.CLIDs, len(requests))
		for i, req := range requests {
			clids[i] = common.CLID(req.Clid)
		}
		clsToCancel, err = changelist.LoadCLsByIDs(ctx, clids)
		return err
	})
	eg.Go(func() error {
		runCLs, err := run.LoadRunCLs(ctx, op.Run.ID, op.Run.CLs)
		if err != nil {
			return err
		}
		triggers = make(map[common.CLID]*run.Triggers, len(runCLs))
		allRunCLExternalIDs = make([]changelist.ExternalID, len(runCLs))
		for i, runCL := range runCLs {
			triggers[runCL.ID] = triggers[runCL.ID].WithTrigger(runCL.Trigger)
			allRunCLExternalIDs[i] = runCL.ExternalID
		}
		return nil
	})
	eg.Go(func() (err error) {
		cfg, err = prjcfg.GetConfigGroup(ctx, op.Run.ID.LUCIProject(), op.Run.ConfigGroupID)
		return err
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	op.inputs = make([]cancel.Input, len(requests))
	op.results = make([]cancelResult, len(requests))
	luciProject := op.Run.ID.LUCIProject()
	for i := range requests {
		cl, req := clsToCancel[i], requests[i]
		op.inputs[i] = cancel.Input{
			CL:                cl,
			Triggers:          triggers[cl.ID],
			LUCIProject:       luciProject,
			Message:           req.Message,
			Requester:         "Trigger Cancellation",
			Notify:            convertToGerritWhoms(req.Notify),
			LeaseDuration:     time.Minute,
			ConfigGroups:      []*prjcfg.ConfigGroup{cfg},
			RunCLExternalIDs:  allRunCLExternalIDs,
			AddToAttentionSet: convertToGerritWhoms(req.AddToAttention),
			AttentionReason:   req.AddToAttentionReason,
			GFactory:          op.GFactory,
			CLMutator:         op.CLMutator,
		}
		op.results[i] = cancelResult{
			err: notAttemptedYetErr,
		}
	}
	return nil
}

func convertToGerritWhoms(whoms []run.OngoingLongOps_Op_TriggersCancellation_Whom) gerrit.Whoms {
	ret := make(gerrit.Whoms, len(whoms))
	for i, whom := range whoms {
		switch whom {
		case run.OngoingLongOps_Op_TriggersCancellation_OWNER:
			ret[i] = gerrit.Owner
		case run.OngoingLongOps_Op_TriggersCancellation_REVIEWERS:
			ret[i] = gerrit.Reviewers
		case run.OngoingLongOps_Op_TriggersCancellation_CQ_VOTERS:
			ret[i] = gerrit.CQVoters
		default:
			panic(fmt.Errorf("unrecognized whom [%s] in trigger cancellation", whom))
		}
	}
	return ret
}

type cancelItem struct {
	index int
	input cancel.Input
}
type cancelResult struct {
	cancelledAt time.Time
	err         error
}

// notAttemptedYetErr is the initial error set in cancelResult.
var notAttemptedYetErr = errors.New("not attempted cancellation yet")

// executeInParallel cancels the triggers of the provided CLs in parallel
// and keeps retrying on transient or alreadyInLease failure until the context
// is done.
func (op *CancelTriggersOp) executeInParallel(ctx context.Context) {
	dc := op.makeDispatcherChannel(ctx)
	for i, input := range op.inputs {
		dc.C <- cancelItem{index: i, input: input}
	}
	dc.Close()
	<-dc.DrainC
}

func (op *CancelTriggersOp) makeDispatcherChannel(ctx context.Context) dispatcher.Channel {
	concurrency := op.CancelConcurrency
	if concurrency == 0 {
		concurrency = defaultCancelConcurrency
	}
	concurrency = min(concurrency, len(op.inputs))
	dc, err := dispatcher.NewChannel(ctx, &dispatcher.Options{
		ErrorFn: func(failedBatch *buffer.Batch, err error) (retry bool) {
			_, isLeaseErr := lease.IsAlreadyInLeaseErr(err)
			return isLeaseErr || transient.Tag.In(err)
		},
		DropFn: dispatcher.DropFnQuiet,
		Buffer: buffer.Options{
			MaxLeases:     concurrency,
			BatchItemsMax: 1,
			FullBehavior: &buffer.BlockNewItems{
				MaxItems: len(op.results),
			},
			Retry: op.makeRetryFactory(),
		},
	}, func(data *buffer.Batch) error {
		ci, ok := data.Data[0].Item.(cancelItem)
		if !ok {
			panic(fmt.Errorf("unexpected batch data item %s", data.Data[0].Item))
		}
		result := &op.results[ci.index]
		result.err = cancel.Cancel(ctx, ci.input)
		if result.err == nil {
			result.cancelledAt = clock.Now(ctx)
		}
		if op.testAfterTryCancelFn != nil {
			op.testAfterTryCancelFn()
		}
		return result.err
	})
	if err != nil {
		panic(fmt.Errorf("unexpected failure when creating dispatcher channel: %s", err))
	}
	return dc
}

func (op *CancelTriggersOp) makeRetryFactory() retry.Factory {
	return func() retry.Iterator {
		return &cancellationRetryIterator{
			inner: &retry.ExponentialBackoff{
				Limited: retry.Limited{
					Delay:   100 * time.Millisecond,
					Retries: -1, // unlimited
				},
				Multiplier: 2,
				MaxDelay:   1 * time.Minute,
			},
		}
	}
}

// cancellationRetryIterator retries on transient failure in an exponential
// fashion. If the error is alreadyInLease, this iterator returns the earliest
// time of the lease expiry and the inner's iterator next retry.
type cancellationRetryIterator struct {
	inner retry.Iterator
}

// Next implements retry.Iterator
func (c cancellationRetryIterator) Next(ctx context.Context, err error) time.Duration {
	switch leaseErr, isLeaseErr := lease.IsAlreadyInLeaseErr(err); {
	case isLeaseErr:
		innerNext := c.inner.Next(ctx, err)
		if timeToExpire := clock.Until(ctx, leaseErr.ExpireTime); timeToExpire > 0 && timeToExpire < innerNext {
			return timeToExpire
		}
		return innerNext
	case transient.Tag.In(err):
		return c.inner.Next(ctx, err)
	default:
		return retry.Stop
	}
}
