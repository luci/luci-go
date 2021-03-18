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

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// OnReadyForSubmission implements Handler interface.
func (*Impl) OnReadyForSubmission(ctx context.Context, rs *state.RunState) (*Result, error) {
	// TODO(yiwzhang): implement
	return &Result{State: rs}, nil
}

// OnCLSubmitted implements Handler interface.
func (*Impl) OnCLSubmitted(ctx context.Context, rs *state.RunState, clids common.CLIDs) (*Result, error) {
	panic("implement")
}

// OnSubmissionCompleted implements Handler interface.
func (*Impl) OnSubmissionCompleted(ctx context.Context, rs *state.RunState, sr eventpb.SubmissionResult, attempt int32) (*Result, error) {
	panic("implement")
}
