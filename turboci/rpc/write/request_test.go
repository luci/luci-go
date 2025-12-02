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

func TestAddReason(t *testing.T) {
	t.Parallel()

	req := write.NewRequest()

	rsn, err := req.AddReason("hello", boolData)
	assert.NoErr(t, err)
	rsn.SetRealm("some/realm")

	_, err = req.AddReason("yo", numData)
	assert.NoErr(t, err)

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		Reasons: []*orchestratorpb.WriteNodesRequest_Reason{
			orchestratorpb.WriteNodesRequest_Reason_builder{
				Reason: proto.String("hello"),
				Realm:  proto.String("some/realm"),
				Details: []*orchestratorpb.Value{
					data.Value(boolData),
				},
			}.Build(),
			orchestratorpb.WriteNodesRequest_Reason_builder{
				Reason: proto.String("yo"),
				Details: []*orchestratorpb.Value{
					data.Value(numData),
				},
			}.Build(),
		},
	}.Build()))
}
