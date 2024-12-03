// Copyright 2023 The LUCI Authors.
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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/postaction"
)

const maxPostActionExecutionDuration = 8 * time.Minute

func (impl *Impl) onCompletedPostAction(ctx context.Context, rs *state.RunState, op *run.OngoingLongOps_Op, opResult *eventpb.LongOpCompleted) (*Result, error) {
	opID := opResult.GetOperationId()
	rs = rs.ShallowCopy()
	rs.RemoveCompletedLongOp(opID)
	summary := opResult.GetExecutePostAction().GetSummary()
	label := fmt.Sprintf("PostAction[%s]", op.GetExecutePostAction().GetName())

	switch st := opResult.GetStatus(); {
	// If there is a summary reported by the executor, add it to the run log,
	// regardless of the stauts. For example, the op could be EXPIRED after
	// a number of votes succeeded.
	case len(summary) > 0:
		rs.LogInfo(ctx, label, summary)

	// If len(summary) == 0, it implies that the long op was completed before
	// the op was dispatched to the PostAction execution handler.
	//
	// The PostAction handler should set Summary if postaction.Executor.Do()
	// is called.
	case st == eventpb.LongOpCompleted_EXPIRED:
		rs.LogInfo(ctx, label, "the execution deadline was exceeded")
	case st == eventpb.LongOpCompleted_FAILED:
		rs.LogInfo(ctx, label, "the execution failed")
	case st == eventpb.LongOpCompleted_CANCELLED:
		rs.LogInfo(ctx, label, "the execution was cancelled")
	case st == eventpb.LongOpCompleted_SUCCEEDED:
		rs.LogInfo(ctx, label, "the execution succeeded")
	default:
		panic(fmt.Errorf("unexpected LongOpCompleted status: %s", st))
	}
	return &Result{State: rs}, nil
}

// enqueueExecutePostActionTask enqueues internal long ops for post action and
// the post action defined in the config, of which triggering conditions are
// met.
//
// The payload is made of the project config associated with the Run
// at the time of this call. Even if a new project config is pushed with new
// PostAction configs, the changes won't affect the ongoing PostActions.
//
// PostActions are executed for ended Runs. New config changes have no impact
// on ended Runs, and new project configs with PostAction shouldn't affect
// already ended Runs and their ongoing PostActions, either.
func enqueueExecutePostActionTask(ctx context.Context, rs *state.RunState, cg *prjcfg.ConfigGroup) {
	// enqueue internal post actions.
	// Do not credit the run quota for on upload runs (NewPatchsetRun).
	if rs.Mode != run.NewPatchsetRun {
		rs.EnqueueLongOp(&run.OngoingLongOps_Op{
			Deadline: timestamppb.New(clock.Now(ctx).UTC().Add(maxPostActionExecutionDuration)),
			Work: &run.OngoingLongOps_Op_ExecutePostAction{
				ExecutePostAction: &run.OngoingLongOps_Op_ExecutePostActionPayload{
					Name: postaction.CreditRunQuotaPostActionName,
					Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_CreditRunQuota_{
						CreditRunQuota: &run.OngoingLongOps_Op_ExecutePostActionPayload_CreditRunQuota{},
					},
				},
			},
		})
	}
	// enqueue actions defined in the configuration.
	for _, pacfg := range cg.Content.GetPostActions() {
		if postaction.IsTriggeringConditionMet(pacfg, &rs.Run) {
			rs.EnqueueLongOp(&run.OngoingLongOps_Op{
				Deadline: timestamppb.New(clock.Now(ctx).UTC().Add(maxPostActionExecutionDuration)),
				Work: &run.OngoingLongOps_Op_ExecutePostAction{
					ExecutePostAction: &run.OngoingLongOps_Op_ExecutePostActionPayload{
						Name: pacfg.GetName(),
						Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_ConfigAction{
							ConfigAction: pacfg,
						},
					},
				},
			})
		}
	}
}
