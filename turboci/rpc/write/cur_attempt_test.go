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

	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/value"
)

func TestCurrentAttempt(t *testing.T) {
	t.Parallel()

	csw := write.CurrentAttemptWrite{Msg: &orchestratorpb.WriteNodesRequest_CurrentAttemptWrite{}}

	csw.AddProgress("hei", value.MustWrite(boolData))

	csw.AddDetails(value.MustWrite(boolData), value.MustWrite(numData))

	assert.That(t, csw.Msg, should.Match(orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_builder{
		Details: []*orchestratorpb.ValueWrite{
			orchestratorpb.ValueWrite_builder{
				Data:  mustAny(boolData),
				Realm: proto.String(value.RealmFromContainer),
			}.Build(),
			orchestratorpb.ValueWrite_builder{
				Data:  mustAny(numData),
				Realm: proto.String(value.RealmFromContainer),
			}.Build(),
		},
		Progress: []*orchestratorpb.WriteNodesRequest_StageAttemptProgress{
			orchestratorpb.WriteNodesRequest_StageAttemptProgress_builder{
				Message: proto.String("hei"),
				Details: []*orchestratorpb.ValueWrite{
					orchestratorpb.ValueWrite_builder{
						Data:  mustAny(boolData),
						Realm: proto.String(value.RealmFromContainer),
					}.Build(),
				},
			}.Build(),
		},
	}.Build()))
}

func TestGetCurrentStage(t *testing.T) {
	t.Parallel()

	req := write.NewRequest()
	cur := req.GetCurrentAttempt()
	cur.AddProgress("hei")

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		CurrentAttempt: orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_builder{
			Progress: []*orchestratorpb.WriteNodesRequest_StageAttemptProgress{
				orchestratorpb.WriteNodesRequest_StageAttemptProgress_builder{
					Message: proto.String("hei"),
				}.Build(),
			},
		}.Build(),
	}.Build()))
}
