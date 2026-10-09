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
	"go.chromium.org/turboci/proto/go/utils/ids"
	"go.chromium.org/turboci/proto/go/utils/value"

	"go.chromium.org/luci/turboci/rpc/write"
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

	cw.AddResultData(value.MustWrite(numData))

	cw.AddResultData(value.MustWrite(boolData, "some/realm"))

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
		ResultData: []*orchestratorpb.ValueWrite{
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

	chk := req.AddNewCheck(ids.Check("something"), orchestratorpb.CheckKind_CHECK_KIND_BUILD)
	chk.AddOptions(value.MustWrite(numData, "some/realm"))

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		Checks: []*orchestratorpb.WriteNodesRequest_CheckWrite{
			orchestratorpb.WriteNodesRequest_CheckWrite_builder{
				Identifier: ids.Check("something"),
				Kind:       orchestratorpb.CheckKind_CHECK_KIND_BUILD.Enum(),
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

func makeCheckAttr(name, expr string) *orchestratorpb.WriteNodesRequest_CheckAttributeWrite {
	return orchestratorpb.WriteNodesRequest_CheckAttributeWrite_builder{
		Name:       proto.String(name),
		Expression: proto.String(expr),
	}.Build()
}

func TestCheckWrite_AddAttributes(t *testing.T) {
	t.Parallel()

	cw := write.CheckWrite{Msg: &orchestratorpb.WriteNodesRequest_CheckWrite{}}
	attr1 := makeCheckAttr("custom.attr1", "tags.check.options.hasKey('foo')")
	attr2 := makeCheckAttr("custom.attr2", "tags.check.result.data.getIntValues('count').first() > 0")

	cw.AddAttributes(attr1)
	cw.AddAttributes(attr2)

	assert.That(t, cw.Msg.GetAttributes(), should.Match([]*orchestratorpb.WriteNodesRequest_CheckAttributeWrite{
		attr1,
		attr2,
	}))
}

func TestAddNewCheck_AddAttributes(t *testing.T) {
	t.Parallel()

	req := write.NewRequest()
	chk := req.AddNewCheck(ids.Check("check_id"), orchestratorpb.CheckKind_CHECK_KIND_BUILD)
	attr := makeCheckAttr("custom.attr", "true")

	chk.AddAttributes(attr)

	assert.That(t, req.Msg, should.Match(orchestratorpb.WriteNodesRequest_builder{
		Checks: []*orchestratorpb.WriteNodesRequest_CheckWrite{
			orchestratorpb.WriteNodesRequest_CheckWrite_builder{
				Identifier: ids.Check("check_id"),
				Kind:       orchestratorpb.CheckKind_CHECK_KIND_BUILD.Enum(),
				Attributes: []*orchestratorpb.WriteNodesRequest_CheckAttributeWrite{
					attr,
				},
			}.Build(),
		},
	}.Build()))
}
