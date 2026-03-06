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
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/check"
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/value"
)

var numData = structpb.NewNumberValue(100.0)
var boolData = structpb.NewBoolValue(true)

func mustAny(msg proto.Message) *anypb.Any {
	ret, err := anypb.New(msg)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestCheckWrite(t *testing.T) {
	t.Parallel()

	cw := write.CheckWrite{Msg: &orchestratorpb.WriteNodesRequest_CheckWrite{}}

	cw.AddOptions(value.MustWrite(numData, "some/realm"))

	cw.AddOptions(value.MustWrite(boolData))

	cw.AddResults(value.MustWrite(numData))

	cw.AddResults(value.MustWrite(boolData, "some/realm"))

	assert.That(t, cw.Msg, should.Match(orchestratorpb.WriteNodesRequest_CheckWrite_builder{
		Options: []*orchestratorpb.ValueWrite{
			orchestratorpb.ValueWrite_builder{
				Data:  mustAny(numData),
				Realm: proto.String("some/realm"),
			}.Build(),
			orchestratorpb.ValueWrite_builder{
				Data:  mustAny(boolData),
				Realm: proto.String(value.RealmFromContainer),
			}.Build(),
		},
		Results: []*orchestratorpb.ValueWrite{
			orchestratorpb.ValueWrite_builder{
				Data:  mustAny(numData),
				Realm: proto.String(value.RealmFromContainer),
			}.Build(),
			orchestratorpb.ValueWrite_builder{
				Data:  mustAny(boolData),
				Realm: proto.String("some/realm"),
			}.Build(),
		},
	}.Build()))
}

func TestCheckAddNew(t *testing.T) {
	t.Parallel()

	// NOTE: CheckAddNew fully covers CheckAddUpdate.

	req := write.NewRequest()

	chk := req.AddNewCheck(id.Check("something"), check.KindBuild)
	chk.AddOptions(value.MustWrite(numData, "some/realm"))

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		Checks: []*orchestratorpb.WriteNodesRequest_CheckWrite{
			orchestratorpb.WriteNodesRequest_CheckWrite_builder{
				Identifier: id.Check("something"),
				Kind:       check.KindBuild.Enum(),
				Options: []*orchestratorpb.ValueWrite{
					orchestratorpb.ValueWrite_builder{
						Data:  mustAny(numData),
						Realm: proto.String("some/realm"),
					}.Build(),
				},
			}.Build(),
		},
	}.Build()))
}
