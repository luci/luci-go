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

package longops

import (
	"context"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/quota"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/postaction"
)

// ExecutePostActionOp executes a PostAction.
type ExecutePostActionOp struct {
	*Base
	GFactory    gerrit.Factory
	RunNotifier *run.Notifier
	QM          *quota.Manager
}

// Do implements Operation interface.
func (op *ExecutePostActionOp) Do(ctx context.Context) (*eventpb.LongOpCompleted, error) {
	if op.IsCancelRequested() {
		return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_CANCELLED}, nil
	}
	exe := postaction.Executor{
		GFactory:          op.GFactory,
		Run:               op.Run,
		Payload:           op.Op.GetExecutePostAction(),
		RM:                op.RunNotifier,
		QM:                op.QM,
		IsCancelRequested: op.IsCancelRequested,
	}
	summary, err := exe.Do(ctx)
	return op.report(ctx, err, summary), errors.WrapIf(
		err, "post-action-%s", exe.Payload.GetName())
}

func (op *ExecutePostActionOp) report(ctx context.Context, err error, summary string) *eventpb.LongOpCompleted {
	var status eventpb.LongOpCompleted_Status
	switch {
	case err == nil:
		status = eventpb.LongOpCompleted_SUCCEEDED
	case op.IsCancelRequested():
		// It's possible that the cancellation was detected after Do() failed
		// by permanent failures. It's still ok to mark the op as cancelled.
		// Cancallation can happen after partial successes and failures anyways.
		status = eventpb.LongOpCompleted_CANCELLED
	case ctx.Err() != nil:
		// Same as CANCELLED. It's ok to mark the op as expired, regardless
		// of the error that Do returned.
		status = eventpb.LongOpCompleted_EXPIRED
	default:
		status = eventpb.LongOpCompleted_FAILED
	}
	return &eventpb.LongOpCompleted{
		Status: status,
		Result: &eventpb.LongOpCompleted_ExecutePostAction{
			ExecutePostAction: &eventpb.LongOpCompleted_ExecutePostActionResult{
				Summary: summary,
			},
		},
	}
}
