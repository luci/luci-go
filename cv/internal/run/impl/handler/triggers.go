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
	"fmt"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

const (
	// maxResetTriggersDuration is the maximum duration allowed for a Run
	// to reset the triggers of all CLs.
	maxResetTriggersDuration = 5 * time.Minute

	logEntryLabelResetTriggers = "Reset Triggers"
)

func (impl *Impl) onCompletedResetTriggers(ctx context.Context, rs *state.RunState, op *run.OngoingLongOps_Op, opCompleted *eventpb.LongOpCompleted) (*Result, error) {
	opID := opCompleted.GetOperationId()
	rs = rs.ShallowCopy()
	rs.RemoveCompletedLongOp(opID)
	if status := rs.Status; run.IsEnded(status) {
		logging.Warningf(ctx, "long operation to reset triggers has completed but Run is %s. Result: %s", rs.Status, opCompleted)
		return &Result{State: rs}, nil
	}
	runStatus := op.GetResetTriggers().GetRunStatusIfSucceeded()
	switch opCompleted.GetStatus() {
	case eventpb.LongOpCompleted_EXPIRED:
		runStatus = run.Status_FAILED
		rs.LogInfof(ctx, logEntryLabelResetTriggers, "failed to reset the triggers of CLs within the %s deadline", maxResetTriggersDuration)
	case eventpb.LongOpCompleted_FAILED:
		runStatus = run.Status_FAILED
		fallthrough
	case eventpb.LongOpCompleted_SUCCEEDED:
		for _, result := range opCompleted.GetResetTriggers().GetResults() {
			changeURL := changelist.ExternalID(result.GetExternalId()).MustURL()
			switch result.GetDetail().(type) {
			case *eventpb.LongOpCompleted_ResetTriggers_Result_SuccessInfo:
				rs.LogInfofAt(result.GetSuccessInfo().GetResetAt().AsTime(), logEntryLabelResetTriggers, "successfully reset the trigger of change %s", changeURL)
				if tjEndTime := rs.Tryjobs.GetState().GetEndTime(); tjEndTime != nil {
					delay := result.GetSuccessInfo().GetResetAt().AsTime().Sub(tjEndTime.AsTime())
					metrics.Internal.RunTryjobResultReportDelay.Add(ctx, float64(delay.Milliseconds()),
						rs.ID.LUCIProject(), rs.ConfigGroupID.Name(), string(rs.Mode))
				}
			case *eventpb.LongOpCompleted_ResetTriggers_Result_FailureInfo:
				rs.LogInfof(ctx, logEntryLabelResetTriggers, "failed to reset the trigger of change %s. Reason: %s", changeURL, result.GetFailureInfo().GetFailureMessage())
			default:
				panic(fmt.Errorf("unexpected long op result status: %s", opCompleted.GetStatus()))
			}
		}
	default:
		panic(fmt.Errorf("unexpected LongOpCompleted status: %s", opCompleted.GetStatus()))
	}
	cg, err := prjcfg.GetConfigGroup(ctx, rs.ID.LUCIProject(), rs.ConfigGroupID)
	if err != nil {
		return nil, errors.Fmt("prjcfg.GetConfigGroup: %w", err)
	}
	childRuns, err := run.LoadChildRuns(ctx, rs.ID)
	if err != nil {
		return nil, errors.Fmt("failed to load child runs: %w", err)
	}
	return &Result{
		State:        rs,
		SideEffectFn: impl.endRun(ctx, rs, runStatus, cg, childRuns),
	}, nil
}
