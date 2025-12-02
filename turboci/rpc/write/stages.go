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

	"google.golang.org/protobuf/proto"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/check"
	"go.chromium.org/luci/turboci/data"
)

// StageWrite wraps an orchestratorpb.WriteNodesRequest_StageWrite.
//
// This notably provides the helpers to add options and results to the
// StageWrite.
//
// See package documentation for the behavior of this wrapper type.
type StageWrite struct {
	Msg *orchestratorpb.WriteNodesRequest_StageWrite
}

// AddCheckAssignment adds a new Assignment to the StageWrite.
func (sw StageWrite) AddCheckAssignment(id *idspb.Check, goalState check.State) *orchestratorpb.Stage_Assignment {
	ret := orchestratorpb.Stage_Assignment_builder{
		Target:    id,
		GoalState: &goalState,
	}.Build()
	sw.Msg.SetAssignments(append(sw.Msg.GetAssignments(), ret))
	return ret
}

// AddNewStage adds a new stage to the WriteNodesRequest.
//
// Only returns an error if `args` cannot be marshalled.
func (req Request) AddNewStage(id *idspb.Stage, args proto.Message) (StageWrite, error) {
	val, err := data.ValueErr(args)
	if err != nil {
		return StageWrite{}, fmt.Errorf("write.StageAddNew: %w", err)
	}

	ret := orchestratorpb.WriteNodesRequest_StageWrite_builder{
		Identifier: id,
		Args:       val,
	}.Build()
	req.Msg.SetStages(append(req.Msg.GetStages(), ret))
	return StageWrite{ret}, nil
}

// AddStageCancellation adds a new stage cancellation to the WriteNodesRequest.
//
// Returns the StageWrite for consistency with other Add methods, but there are
// no additional meaningful fields to change on the StageWrite when cancelling
// a Stage.
func (req Request) AddStageCancellation(id *idspb.Stage) StageWrite {
	ret := orchestratorpb.WriteNodesRequest_StageWrite_builder{
		Identifier: id,
		Cancelled:  proto.Bool(true),
	}.Build()
	req.Msg.SetStages(append(req.Msg.GetStages(), ret))
	return StageWrite{ret}
}
