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
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/postaction"
)

// ExecutePostActionOp executes a PostAction.
type ExecutePostActionOp struct {
	*Base
	GFactory gerrit.Factory
}

// Do implements Operation interface.
func (op *ExecutePostActionOp) Do(ctx context.Context) (*eventpb.LongOpCompleted, error) {
	exe := postaction.NewExecutor(op.GFactory)
	err := exe.Do(ctx, op.Op.GetExecutePostAction())
	if err == nil {
		return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_SUCCEEDED}, nil
	}
	return nil, errors.Annotate(err, "postaction.Executor.Do").Err()
}
