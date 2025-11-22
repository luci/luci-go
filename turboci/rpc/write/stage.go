// Copyright 2025 The LUCI Authors.
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

package write

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/proto/delta"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write/stage"
)

// NewStage returns a Diff which adds a new Stage to the write.
//
// If not changed via an diff, the Stage will:
//   - inherit the current Stage's realm
//   - be inserted into the current Stage's workplan
//   - have the Stage's `is_worknode` automatically inferred from the type of
//     `args` (it will be assumed to be a normal stage, unless this type ends with
//     ".WorkNode").
func NewStage(stageID string, args proto.Message, diffs ...*stage.Diff) *Diff {
	sw := &orchestratorpb.WriteNodesRequest_StageWrite{}
	var errs []error

	argsVal, err := data.ValueErr(args)
	if err != nil {
		errs = append(errs, fmt.Errorf("write.NewStage.args: %w", err))
	}
	sw.SetArgs(argsVal)
	mode := id.StageNotWorknode
	if strings.HasSuffix(argsVal.GetValue().TypeUrl, ".WorkNode") {
		mode = id.StageIsWorknode
	}
	sid, err := id.StageErr(mode, stageID)
	if err != nil {
		errs = append(errs, fmt.Errorf("write.NewStage.stageID: %w", err))
	}
	sw.SetIdentifier(sid)
	if err := delta.CollectInto(sw, diffs...); err != nil {
		errs = append(errs, fmt.Errorf("write.NewStage: %w", err))
	}
	return template.New(builder{
		Stages: []*orchestratorpb.WriteNodesRequest_StageWrite{sw},
	}, errs...)
}

// CancelStage returns a Diff which adds a Stage cancellation to the write.
//
//	`workplanID` is meant to be an optional argument; It should be provided
//
// either 0 or 1 times. Providing it more than once is an error.
func CancelStage(stageID string, isWorknode bool, workplanID ...string) *Diff {
	sw := &orchestratorpb.WriteNodesRequest_StageWrite{}
	sw.SetCancelled(true)
	var errs []error
	mode := id.StageNotWorknode
	if isWorknode {
		mode = id.StageIsWorknode
	}
	sid, err := id.StageErr(mode, stageID)
	if err != nil {
		errs = append(errs, fmt.Errorf("write.CancelStage.stageID: %w", err))
	}
	if len(workplanID) == 1 {
		if _, err := id.SetWorkplanErr(sid, workplanID[0]); err != nil {
			errs = append(errs, fmt.Errorf("write.CancelStage.workplanID: %w", err))
		}
	} else if len(workplanID) > 1 {
		errs = append(errs, fmt.Errorf("write.CancelStage.workplanID: provided more than once: len == %d", len(workplanID)))
	}
	sw.SetIdentifier(sid)
	return template.New(builder{
		Stages: []*orchestratorpb.WriteNodesRequest_StageWrite{sw},
	}, errs...)
}
