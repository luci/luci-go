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

func (impl *Impl) onCompletedPostAction(ctx context.Context, rs *state.RunState, op *run.OngoingLongOps_Op, opCompleted *eventpb.LongOpCompleted) (*Result, error) {
	opID := opCompleted.GetOperationId()
	rs = rs.ShallowCopy()
	rs.RemoveCompletedLongOp(opID)
	// TODO(ddoman): handle errors
	return &Result{State: rs}, nil
}

// enqueueExecutePostActionTask enqueus long-ops for the PostActions, of which
// triggering conditions are met.
//
// The payload is made of the project config associated with the Run
// at the time of this call. Even if a new project config is pushed with new
// PostAction configs, the changes won't affect the ongoing PostActions.
//
// PostActions are executed for ended Runs. New config changes have no impact
// on ended Runs, and new project configs with PostAction shouldn't affect
// already ended Runs and their ongoing PostActions, either.
func enqueueExecutePostActionTask(ctx context.Context, rs *state.RunState, cg *prjcfg.ConfigGroup) {
	for _, pacfg := range cg.Content.GetPostActions() {
		if postaction.IsTriggeringConditionMet(pacfg, &rs.Run) {
			rs.EnqueueLongOp(&run.OngoingLongOps_Op{
				Deadline: timestamppb.New(clock.Now(ctx).UTC().Add(maxTryjobExecutorDuration)),
				Work: &run.OngoingLongOps_Op_ExecutePostAction{
					ExecutePostAction: &run.OngoingLongOps_Op_ExecutePostActionPayload{
						Action: pacfg,
					},
				},
			})
		}
	}
}
