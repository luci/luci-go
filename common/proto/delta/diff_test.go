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

package delta_test

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/proto/delta"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/id"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

type (
	testMsg        = orchestratorpb.WriteNodesRequest_CheckWrite
	testMsgBuilder = orchestratorpb.WriteNodesRequest_CheckWrite_builder
)

var testTemplate = delta.MakeTemplate[testMsgBuilder](map[string]delta.ApplyMode{
	"identifier": delta.ModeMerge,
	"state":      delta.ModeMaxEnum,
})

func TestDiff(t *testing.T) {
	t.Run(`ModeDefault`, func(t *testing.T) {
		t.Run(`scalar`, func(t *testing.T) {
			diff := testTemplate.New(testMsgBuilder{
				FinalizeResults: proto.Bool(true),
			})
			msg := &testMsg{}
			assert.NoErr(t, diff.Apply(msg))
			assert.Loosely(t, msg.GetFinalizeResults(), should.Equal(true))
		})

		t.Run(`repeated`, func(t *testing.T) {
			diff := testTemplate.New(testMsgBuilder{
				Options: []*orchestratorpb.WriteNodesRequest_RealmValue{
					orchestratorpb.WriteNodesRequest_RealmValue_builder{
						Value: data.Value(structpb.NewStringValue("hi there")),
					}.Build(),
				},
			})
			msg := testMsgBuilder{
				Options: []*orchestratorpb.WriteNodesRequest_RealmValue{
					orchestratorpb.WriteNodesRequest_RealmValue_builder{
						Value: data.Value(structpb.NewNumberValue(12345)),
					}.Build(),
				},
			}.Build()
			assert.NoErr(t, diff.Apply(msg))
			assert.That(t, msg.GetOptions(), should.Match(
				[]*orchestratorpb.WriteNodesRequest_RealmValue{
					orchestratorpb.WriteNodesRequest_RealmValue_builder{
						Value: data.Value(structpb.NewNumberValue(12345)),
					}.Build(),
					orchestratorpb.WriteNodesRequest_RealmValue_builder{
						Value: data.Value(structpb.NewStringValue("hi there")),
					}.Build(),
				},
			))
		})
	})

	t.Run(`ModeMaxEnum`, func(t *testing.T) {
		diff := testTemplate.New(testMsgBuilder{
			State: orchestratorpb.CheckState_CHECK_STATE_FINAL.Enum(),
		})
		msg := testMsgBuilder{
			State: orchestratorpb.CheckState_CHECK_STATE_WAITING.Enum(),
		}.Build()
		assert.NoErr(t, diff.Apply(msg))
		assert.That(
			t, msg.GetState(),
			should.Equal(orchestratorpb.CheckState_CHECK_STATE_FINAL))

		// now try to move it backwards to WAITING
		assert.NoErr(t, diff.Apply(msg))
		assert.That(t, msg.GetState(), should.Equal(orchestratorpb.CheckState_CHECK_STATE_FINAL))
	})

	t.Run(`ModeMerge`, func(t *testing.T) {
		diff := testTemplate.New(testMsgBuilder{
			Identifier: idspb.Check_builder{
				WorkPlan: idspb.WorkPlan_builder{
					Id: proto.String("workplan-id"),
				}.Build(),
			}.Build(),
		})

		msg := testMsgBuilder{
			Identifier: id.Check("hello"),
		}.Build()
		assert.NoErr(t, diff.Apply(msg))

		assert.That(t, msg.GetIdentifier(), should.Match(idspb.Check_builder{
			WorkPlan: idspb.WorkPlan_builder{
				Id: proto.String("workplan-id"),
			}.Build(),
			Id: proto.String("hello"),
		}.Build()))
	})

	t.Run(`can apply nil`, func(t *testing.T) {
		msg := testMsgBuilder{
			Identifier: id.Check("hello"),
		}.Build()
		assert.NoErr(t, ((*delta.Diff[*testMsg])(nil)).Apply(msg))
		assert.That(t, msg, should.Match(testMsgBuilder{
			Identifier: id.Check("hello"),
		}.Build()))
	})
}

func TestCollectDiffs(t *testing.T) {
	msg, err := delta.Collect(
		testTemplate.New(testMsgBuilder{
			State: orchestratorpb.CheckState_CHECK_STATE_PLANNED.Enum(),
		}),
		testTemplate.New(testMsgBuilder{
			Identifier: id.Check("whats up"),
		}),
		testTemplate.New(testMsgBuilder{
			Identifier: idspb.Check_builder{
				WorkPlan: idspb.WorkPlan_builder{
					Id: proto.String("workplan-id"),
				}.Build(),
			}.Build(),
		}),
	)
	assert.NoErr(t, err)
	assert.That(t, msg, should.Match(testMsgBuilder{
		State: orchestratorpb.CheckState_CHECK_STATE_PLANNED.Enum(),
		Identifier: idspb.Check_builder{
			WorkPlan: idspb.WorkPlan_builder{
				Id: proto.String("workplan-id"),
			}.Build(),
			Id: proto.String("whats up"),
		}.Build(),
	}.Build()))
}

func TestAbsorb(t *testing.T) {
	diff := delta.Combine(
		testTemplate.New(testMsgBuilder{
			State: orchestratorpb.CheckState_CHECK_STATE_PLANNED.Enum(),
		}),
		nil,                     // nil is OK
		&delta.Diff[*testMsg]{}, // empty is OK
		testTemplate.New(testMsgBuilder{
			Identifier: id.Check("whats up"),
		}),
		testTemplate.New(testMsgBuilder{
			Identifier: idspb.Check_builder{
				WorkPlan: idspb.WorkPlan_builder{
					Id: proto.String("workplan-id"),
				}.Build(),
			}.Build(),
		}),
	)

	msg, err := delta.Collect(diff)
	assert.NoErr(t, err)
	assert.That(t, msg, should.Match(testMsgBuilder{
		State: orchestratorpb.CheckState_CHECK_STATE_PLANNED.Enum(),
		Identifier: idspb.Check_builder{
			WorkPlan: idspb.WorkPlan_builder{
				Id: proto.String("workplan-id"),
			}.Build(),
		}.Build(),
	}.Build()))
}
