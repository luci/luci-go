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
	"go.chromium.org/turboci/proto/go/utils/ids"
	"go.chromium.org/turboci/proto/go/utils/value"

	"go.chromium.org/luci/turboci/rpc/write"
)

func TestStageWrite(t *testing.T) {
	t.Parallel()

	sw := write.StageWrite{Msg: &orchestratorpb.WriteNodesRequest_StageWrite{}}

	assigned := sw.AddCheckAssignment(ids.Check("nerp"), orchestratorpb.CheckState_CHECK_STATE_PLANNED)
	assert.That(t, assigned, should.Match(orchestratorpb.Stage_Assignment_builder{
		Target:    ids.Check("nerp"),
		GoalState: orchestratorpb.CheckState_CHECK_STATE_PLANNED.Enum(),
	}.Build()))

	assert.That(t, sw.Msg, should.Match(orchestratorpb.WriteNodesRequest_StageWrite_builder{
		Assignments: []*orchestratorpb.Stage_Assignment{
			orchestratorpb.Stage_Assignment_builder{
				Target:    ids.Check("nerp"),
				GoalState: orchestratorpb.CheckState_CHECK_STATE_PLANNED.Enum(),
			}.Build(),
		},
	}.Build()))
}

func TestAddNewStage(t *testing.T) {
	t.Parallel()

	req := write.NewRequest()
	stg := req.AddNewStage(ids.Stage("stg"), boolData)

	stg.AddCheckAssignment(ids.Check("nerp"), orchestratorpb.CheckState_CHECK_STATE_WAITING)

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		Stages: []*orchestratorpb.WriteNodesRequest_StageWrite{
			orchestratorpb.WriteNodesRequest_StageWrite_builder{
				Identifier: ids.Stage("stg"),
				Args:       value.MustWrite(boolData, value.RealmFromContainer),
				Assignments: []*orchestratorpb.Stage_Assignment{
					orchestratorpb.Stage_Assignment_builder{
						Target:    ids.Check("nerp"),
						GoalState: orchestratorpb.CheckState_CHECK_STATE_WAITING.Enum(),
					}.Build(),
				},
			}.Build(),
		},
	}.Build()))
}

func TestAddStageCancellation(t *testing.T) {
	t.Parallel()

	req := write.NewRequest()
	req.AddStageCancellation(ids.Stage("stg"))

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		StageCancellations: []*orchestratorpb.WriteNodesRequest_StageCancellation{
			orchestratorpb.WriteNodesRequest_StageCancellation_builder{
				Identifier: ids.Stage("stg"),
			}.Build(),
		},
	}.Build()))
}

func makeStageAttr(name, expr string) *orchestratorpb.WriteNodesRequest_StageAttributeWrite {
	return orchestratorpb.WriteNodesRequest_StageAttributeWrite_builder{
		Name:       proto.String(name),
		Expression: proto.String(expr),
	}.Build()
}

func TestStageWrite_AddAttributes(t *testing.T) {
	t.Parallel()

	sw := write.StageWrite{Msg: &orchestratorpb.WriteNodesRequest_StageWrite{}}
	attr1 := makeStageAttr("custom.attr1", "stage.concluded_reason == 'SUCCESS'")
	attr2 := makeStageAttr("custom.attr2", "tags.stage.attempt.detail.hasKey('bar')")

	sw.AddAttributes(attr1)
	sw.AddAttributes(attr2)

	assert.That(t, sw.Msg.GetAttributes(), should.Match([]*orchestratorpb.WriteNodesRequest_StageAttributeWrite{
		attr1,
		attr2,
	}))
}

func TestAddNewStage_AddAttributes(t *testing.T) {
	t.Parallel()

	req := write.NewRequest()
	stg := req.AddNewStage(ids.Stage("stage_id"), boolData)
	attr := makeStageAttr("custom.attr", "stage.concluded_reason == 'SUCCESS'")

	stg.AddAttributes(attr)

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		Stages: []*orchestratorpb.WriteNodesRequest_StageWrite{
			orchestratorpb.WriteNodesRequest_StageWrite_builder{
				Identifier: ids.Stage("stage_id"),
				Args:       value.MustWrite(boolData, value.RealmFromContainer),
				Attributes: []*orchestratorpb.WriteNodesRequest_StageAttributeWrite{
					attr,
				},
			}.Build(),
		},
	}.Build()))
}
