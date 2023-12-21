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

	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

const (
	// maxPostMessageDuration is the max time that a Run will be waiting for
	// a message to be posted on every CL.
	maxPostMessageDuration = 8 * time.Minute

	logEntryLabelPostGerritMessage = "Posting Gerrit Message"
)

func (impl *Impl) onCompletedPostGerritMessage(ctx context.Context, rs *state.RunState, op *run.OngoingLongOps_Op, result *eventpb.LongOpCompleted) (*Result, error) {
	opID := result.GetOperationId()
	rs = rs.ShallowCopy()
	rs.RemoveCompletedLongOp(opID)

	switch result.GetStatus() {
	case eventpb.LongOpCompleted_FAILED:
		rs.LogInfof(ctx, logEntryLabelPostGerritMessage, "Failed to post gerrit message: %s", op.GetPostGerritMessage().GetMessage())
	case eventpb.LongOpCompleted_EXPIRED:
		rs.LogInfo(ctx, logEntryLabelPostGerritMessage, fmt.Sprintf("Failed to post the message to gerrit within the %s deadline", maxPostMessageDuration))
	case eventpb.LongOpCompleted_SUCCEEDED:
		rs.LogInfofAt(result.GetPostGerritMessage().GetTime().AsTime(), logEntryLabelPostGerritMessage, "posted the gerrit message on each CL: %s", op.GetPostGerritMessage().GetMessage())
	case eventpb.LongOpCompleted_CANCELLED:
		rs.LogInfo(ctx, logEntryLabelPostGerritMessage, "cancelled posting gerrit message on CL(s)")
	default:
		panic(fmt.Errorf("unexpected LongOpCompleted status: %s", result.GetStatus()))
	}

	return &Result{State: rs}, nil
}
