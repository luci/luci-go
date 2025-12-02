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

package write_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/check"
	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write"
)

func TestStageWrite(t *testing.T) {
	t.Parallel()

	sw := write.StageWrite{Msg: &orchestratorpb.WriteNodesRequest_StageWrite{}}

	assigned := sw.AddCheckAssignment(id.Check("nerp"), check.StatePlanned)
	assert.That(t, assigned, should.Match(orchestratorpb.Stage_Assignment_builder{
		Target:    id.Check("nerp"),
		GoalState: check.StatePlanned.Enum(),
	}.Build()))

	assert.That(t, sw.Msg, should.Match(orchestratorpb.WriteNodesRequest_StageWrite_builder{
		Assignments: []*orchestratorpb.Stage_Assignment{
			orchestratorpb.Stage_Assignment_builder{
				Target:    id.Check("nerp"),
				GoalState: check.StatePlanned.Enum(),
			}.Build(),
		},
	}.Build()))
}

func TestAddNewStage(t *testing.T) {
	t.Parallel()

	req := write.NewRequest()
	stg, err := req.AddNewStage(id.Stage("stg"), boolData)
	assert.NoErr(t, err)

	stg.AddCheckAssignment(id.Check("nerp"), check.StateWaiting)

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		Stages: []*orchestratorpb.WriteNodesRequest_StageWrite{
			orchestratorpb.WriteNodesRequest_StageWrite_builder{
				Identifier: id.Stage("stg"),
				Args:       data.Value(boolData),
				Assignments: []*orchestratorpb.Stage_Assignment{
					orchestratorpb.Stage_Assignment_builder{
						Target:    id.Check("nerp"),
						GoalState: check.StateWaiting.Enum(),
					}.Build(),
				},
			}.Build(),
		},
	}.Build()))
}

func TestAddStageCancellation(t *testing.T) {
	t.Parallel()

	req := write.NewRequest()
	req.AddStageCancellation(id.Stage("stg"))

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		Stages: []*orchestratorpb.WriteNodesRequest_StageWrite{
			orchestratorpb.WriteNodesRequest_StageWrite_builder{
				Identifier: id.Stage("stg"),
				Cancelled:  proto.Bool(true),
			}.Build(),
		},
	}.Build()))
}
