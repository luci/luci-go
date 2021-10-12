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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// OnLongOpCompleted implements Handler interface.
func (impl *Impl) OnLongOpCompleted(ctx context.Context, rs *state.RunState, result *eventpb.LongOpCompleted) (*Result, error) {
	switch runStatus := rs.Status; {
	case run.IsEnded(runStatus):
		logging.Debugf(ctx, "Ignoring %s long operation %q because Run is %s", result.GetStatus(), result.GetOperationId(), runStatus)
		return &Result{State: rs}, nil
	case runStatus == run.Status_PENDING:
		return nil, errors.Reason("expected at least RUNNING status, got %s", runStatus).Err()
	default:
		// TODO handle.
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
	now := clock.Now(ctx)
	for opID, op := range rs.OngoingLongOps.GetOps() {
		if op.GetDeadline().AsTime().Before(now) {
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
