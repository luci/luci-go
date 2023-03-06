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

package handler

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// longOpGracePeriod is additional time waited for the long operation
// completion event to be received before force-expiring it.
const longOpGracePeriod = time.Minute

// OnLongOpCompleted implements Handler interface.
func (impl *Impl) OnLongOpCompleted(ctx context.Context, rs *state.RunState, result *eventpb.LongOpCompleted) (*Result, error) {
	op := rs.OngoingLongOps.GetOps()[result.GetOperationId()]
	if op == nil {
		logging.Warningf(ctx, "Long operation %q has no entry in Run (maybe already expired?)", result.GetOperationId())
		return &Result{State: rs}, nil
	}

	switch w := op.GetWork().(type) {
	case *run.OngoingLongOps_Op_PostStartMessage:
		return impl.onCompletedPostStartMessage(ctx, rs, op, result)
	case *run.OngoingLongOps_Op_ResetTriggers_:
		return impl.onCompletedResetTriggers(ctx, rs, op, result)
	case *run.OngoingLongOps_Op_ExecuteTryjobs:
		return impl.onCompletedExecuteTryjobs(ctx, rs, op, result)
	case *run.OngoingLongOps_Op_ExecutePostAction:
		return impl.onCompletedPostAction(ctx, rs, op, result)
	default:
		logging.Errorf(ctx, "Unknown long operation %q work type %T finished with:\n%s", result.GetOperationId(), w, result)
		// Remove the long op from the Run anyway, and move on.
		rs = rs.ShallowCopy()
		rs.RemoveCompletedLongOp(result.GetOperationId())
		return &Result{State: rs}, nil
	}
}

// processExpiredLongOps checks for and handles any long operations whose
// deadline has passed.
//
// Normally, a long operation is expected to send LongOpCompleted event before
// the deadline, which is then processed by OnLongOpCompleted() and ultimately
// removed from the Run.OngoingLongOps.
//
// processExpiredLongOps is a fail-safe for abnormal cases to ensure that a Run
// doesn't remain stuck.
func (impl *Impl) processExpiredLongOps(ctx context.Context, rs *state.RunState) (*Result, error) {
	cutoff := clock.Now(ctx).Add(-longOpGracePeriod)
	for opID, op := range rs.OngoingLongOps.GetOps() {
		if op.GetDeadline().AsTime().Before(cutoff) {
			logging.Warningf(ctx, "Long operation %q has expired at %s", opID, op.GetDeadline().AsTime())
			// In practice, there should be at most 1 ongoing long op.
			// TODO(tandrii): once `Result` objects can be combined, process all
			// expired long ops at once.
			return impl.OnLongOpCompleted(ctx, rs, &eventpb.LongOpCompleted{
				OperationId: opID,
				Status:      eventpb.LongOpCompleted_EXPIRED,
			})
		}
	}
	return &Result{State: rs}, nil
}
