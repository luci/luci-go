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
	"google.golang.org/genproto/googleapis/rpc/code"
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
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

// ResetTriggersOp resets the triggers for the provided CLs.
//
// ResetTriggersOp keeps retrying on lease error and transient failure for each
// CL till the long op deadline is exceeded or reset either succeeds
// or fails non-transiently.
//
// ResetTriggersOp doesn't obey longop's cancellation request because if
// this long op is left half-done, for example, triggers on half of the CLs are
// untouched, a new Run may be created for those CLs.
//
// ResetTriggersOp is a single-use object.
type ResetTriggersOp struct {
	*Base
	GFactory  gerrit.Factory
	CLMutator *changelist.Mutator
	// Concurrency is the number of CLs that will be reset concurrently.
	//
	// Default is 8.
	Concurrency int

	// Private fields that will be populated internally during long op execution.

	inputs  []trigger.ResetInput
	results []resetResult

	// testAfterTryResetFn is always called after each try to reset the trigger
	// of a CL.
	//
	// This is only set for testing purpose.
	testAfterTryResetFn func()
}

const defaultConcurrency = 8

// Do actually resets the triggers.
func (op *ResetTriggersOp) Do(ctx context.Context) (*eventpb.LongOpCompleted, error) {
	op.assertCalledOnce()

	if err := op.loadInputs(ctx); err != nil {
		return nil, err
	}

	op.executeInParallel(ctx)

	longOpStatus := eventpb.LongOpCompleted_SUCCEEDED // be optimistic
	rt := &eventpb.LongOpCompleted_ResetTriggers{
		Results: make([]*eventpb.LongOpCompleted_ResetTriggers_Result, len(op.results)),
	}
	var lastTransErr, lastPermErr error
	for i, result := range op.results {
		cl := op.inputs[i].CL
		rt.Results[i] = &eventpb.LongOpCompleted_ResetTriggers_Result{
			Id:         int64(cl.ID),
			ExternalId: string(cl.ExternalID),
		}
		switch err := result.err; {
		case err == nil:
			rt.Results[i].Detail = &eventpb.LongOpCompleted_ResetTriggers_Result_SuccessInfo{
				SuccessInfo: &eventpb.LongOpCompleted_ResetTriggers_Result_Success{
					ResetAt: timestamppb.New(result.resetAt),
				},
			}
		default:
			longOpStatus = eventpb.LongOpCompleted_FAILED
			rt.Results[i].Detail = &eventpb.LongOpCompleted_ResetTriggers_Result_FailureInfo{
				FailureInfo: &eventpb.LongOpCompleted_ResetTriggers_Result_Failure{
					FailureMessage: err.Error(),
				},
			}
			logging.Errorf(ctx, "failed to reset the trigger of CL %d %q: %s", cl.ID, cl.ExternalID, err)
			if transient.Tag.In(err) {
				lastTransErr = err
			} else {
				lastPermErr = err
			}
		}
	}
	ret := &eventpb.LongOpCompleted{
		Status: longOpStatus,
		Result: &eventpb.LongOpCompleted_ResetTriggers_{
			ResetTriggers: rt,
		},
	}
	switch ctxErr := ctx.Err(); {
	// Returns the event in error case as well because the event will be
	// reported back to Run Manager.
	case ctxErr == context.DeadlineExceeded:
		logging.Errorf(ctx, "running out of time to reset triggers")
		return ret, ctxErr
	case ctxErr == context.Canceled:
		logging.Errorf(ctx, "context is cancelled while resetting triggers")
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

func (op *ResetTriggersOp) loadInputs(ctx context.Context) error {
	var (
		clsToReset []*changelist.CL
		triggers   map[common.CLID]*run.Triggers
		cfg        *prjcfg.ConfigGroup
	)
	eg, ctx := errgroup.WithContext(ctx)
	requests := op.Op.GetResetTriggers().GetRequests()
	eg.Go(func() (err error) {
		clids := make(common.CLIDs, len(requests))
		for i, req := range requests {
			clids[i] = common.CLID(req.Clid)
		}
		clsToReset, err = changelist.LoadCLsByIDs(ctx, clids)
		return err
	})
	eg.Go(func() error {
		runCLs, err := run.LoadRunCLs(ctx, op.Run.ID, op.Run.CLs)
		if err != nil {
			return err
		}
		triggers = make(map[common.CLID]*run.Triggers, len(runCLs))
		for _, runCL := range runCLs {
			if runCL.Trigger != nil {
				triggers[runCL.ID] = triggers[runCL.ID].WithTrigger(runCL.Trigger)
			}
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

	op.inputs = make([]trigger.ResetInput, len(requests))
	op.results = make([]resetResult, len(requests))
	luciProject := op.Run.ID.LUCIProject()
	for i := range requests {
		cl, req := clsToReset[i], requests[i]
		tr, ok := triggers[cl.ID]
		if !ok {
			return errors.Reason("requested trigger reset on CL %d that doesn't have trigger at all", cl.ID).Err()
		}
		op.inputs[i] = trigger.ResetInput{
			CL:                cl,
			Triggers:          tr,
			LUCIProject:       luciProject,
			Message:           req.Message,
			Requester:         "Trigger Reset",
			Notify:            req.Notify,
			LeaseDuration:     time.Minute,
			ConfigGroups:      []*prjcfg.ConfigGroup{cfg},
			AddToAttentionSet: req.AddToAttention,
			AttentionReason:   req.AddToAttentionReason,
			GFactory:          op.GFactory,
			CLMutator:         op.CLMutator,
		}
		op.results[i] = resetResult{
			err: errNotAttemptedYet,
		}
	}
	return nil
}

type resetItem struct {
	index int
	input trigger.ResetInput
}
type resetResult struct {
	resetAt time.Time
	err     error
}

// errNotAttemptedYet is the initial error set in resetResult.
var errNotAttemptedYet = errors.New("not attempted reset yet")

// executeInParallel resets the triggers of the provided CLs in parallel
// and keeps retrying on transient or alreadyInLease failure until the context
// is done.
func (op *ResetTriggersOp) executeInParallel(ctx context.Context) {
	dc := op.makeDispatcherChannel(ctx)
	for i, input := range op.inputs {
		dc.C <- resetItem{index: i, input: input}
	}
	dc.Close()
	<-dc.DrainC
}

func (op *ResetTriggersOp) makeDispatcherChannel(ctx context.Context) dispatcher.Channel {
	concurrency := op.Concurrency
	if concurrency == 0 {
		concurrency = defaultConcurrency
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
		ci, ok := data.Data[0].Item.(resetItem)
		if !ok {
			panic(fmt.Errorf("unexpected batch data item %s", data.Data[0].Item))
		}
		result := &op.results[ci.index]
		result.err = trigger.Reset(ctx, ci.input)
		gerritErr := "GERRIT_ERROR_NONE"
		if errCode, ok := trigger.IsResetErrFromGerrit(result.err); ok {
			if codeString, ok := code.Code_name[int32(errCode)]; ok {
				gerritErr = codeString
			} else {
				gerritErr = fmt.Sprintf("Code(%d)", int64(errCode))
			}
		}
		metrics.Internal.RunResetTriggerAttempted.Add(ctx, 1, op.Run.ID.LUCIProject(), op.Run.ConfigGroupID.Name(), string(op.Run.Mode), result.err == nil, gerritErr)
		if result.err == nil {
			result.resetAt = clock.Now(ctx)
		}
		if op.testAfterTryResetFn != nil {
			op.testAfterTryResetFn()
		}
		return result.err
	})
	if err != nil {
		panic(fmt.Errorf("unexpected failure when creating dispatcher channel: %s", err))
	}
	return dc
}

func (op *ResetTriggersOp) makeRetryFactory() retry.Factory {
	return lease.RetryIfLeased(transient.Only(func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   100 * time.Millisecond,
				Retries: -1, // unlimited
			},
			Multiplier: 2,
			MaxDelay:   1 * time.Minute,
		}
	}))
}
