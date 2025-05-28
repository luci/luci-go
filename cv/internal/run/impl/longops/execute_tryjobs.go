// Copyright 2022 The LUCI Authors.
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
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/tryjob/execute"
)

// ExecuteTryjobsOp executes the Tryjob Requirement for the given Run.
type ExecuteTryjobsOp struct {
	*Base
	Env         *common.Env
	RunNotifier *run.Notifier
	Backend     execute.TryjobBackend
}

// Do implements Operation interface.
func (op *ExecuteTryjobsOp) Do(ctx context.Context) (*eventpb.LongOpCompleted, error) {
	executor := &execute.Executor{
		Env:        op.Env,
		Backend:    op.Backend,
		RM:         op.RunNotifier,
		ShouldStop: op.IsCancelRequested,
	}
	switch err := executor.Do(ctx, op.Run, op.Op.GetExecuteTryjobs()); {
	case err == nil:
		return &eventpb.LongOpCompleted{
			Status: eventpb.LongOpCompleted_SUCCEEDED,
		}, nil
	case !transient.Tag.In(err):
		errors.Log(ctx, errors.Fmt("tryjob executor permanently failed: %w", err))
		fallthrough
	default:
		return nil, err
	}
}
