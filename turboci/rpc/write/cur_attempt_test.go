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

	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/rpc/write"
)

func TestCurrentStageWrite(t *testing.T) {
	t.Parallel()

	csw := write.CurrentStageWrite{Msg: &orchestratorpb.WriteNodesRequest_CurrentStageWrite{}}

	_, err := csw.AddProgress("hei", boolData)
	assert.NoErr(t, err)

	_, err = csw.AddDetails(boolData, numData)
	assert.NoErr(t, err)

	assert.That(t, csw.Msg, should.Match(orchestratorpb.WriteNodesRequest_CurrentStageWrite_builder{
		Details: []*orchestratorpb.Value{
			data.Value(boolData),
			data.Value(numData),
		},
		Progress: []*orchestratorpb.WriteNodesRequest_StageAttemptProgress{
			orchestratorpb.WriteNodesRequest_StageAttemptProgress_builder{
				Msg: proto.String("hei"),
				Details: []*orchestratorpb.Value{
					data.Value(boolData),
				},
			}.Build(),
		},
	}.Build()))
}

func TestGetCurrentStage(t *testing.T) {
	t.Parallel()

	req := write.NewRequest()
	cur := req.GetCurrentStage()
	_, err := cur.AddProgress("hei")
	assert.NoErr(t, err)

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		CurrentStage: orchestratorpb.WriteNodesRequest_CurrentStageWrite_builder{
			Progress: []*orchestratorpb.WriteNodesRequest_StageAttemptProgress{
				orchestratorpb.WriteNodesRequest_StageAttemptProgress_builder{
					Msg: proto.String("hei"),
				}.Build(),
			},
		}.Build(),
	}.Build()))
}
