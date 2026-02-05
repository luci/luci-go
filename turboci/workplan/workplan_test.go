// Copyright 2026 The LUCI Authors.
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

package workplan

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/id"
)

func TestNodeBag(t *testing.T) {
	t.Parallel()

	t.Run(`empty`, func(t *testing.T) {
		t.Parallel()
		nb := ToNodeBag()

		assert.Loosely(t,
			nb.Check(id.SetWorkplan(id.Check("somecheck"), "someplan")), should.BeNil)

		stageID := id.SetWorkplan(id.Stage("somestage"), "someplan")

		assert.Loosely(t, nb.Stage(stageID), should.BeNil)

		assert.Loosely(t, nb.StageAttempt(idspb.StageAttempt_builder{
			Stage: stageID,
			Idx:   proto.Int32(1),
		}.Build()), should.BeNil)
	})

	t.Run(`with_nodes`, func(t *testing.T) {
		cCheck := orchestratorpb.Check_builder{
			Identifier: id.SetWorkplan(id.Check("C"), "wp"),
		}.Build()

		ds, err := data.CheckOptionDataSetFromSlice(cCheck.GetIdentifier(), cCheck.GetOptions())
		assert.NoErr(t, err)
		ds.Add(orchestratorpb.WriteNodesRequest_RealmValue_builder{
			Realm: proto.String("project:realm"),
			Value: data.Value(&emptypb.Empty{}),
		}.Build())
		cCheck.SetOptions(ds.ToSlice())

		opID := cCheck.GetOptions()[0].GetIdentifier()

		nb := ToNodeBag(
			orchestratorpb.WorkPlan_builder{
				Checks: []*orchestratorpb.Check{
					orchestratorpb.Check_builder{
						Identifier: id.SetWorkplan(id.Check("A"), "wp"),
					}.Build(),
					orchestratorpb.Check_builder{
						Identifier: id.SetWorkplan(id.Check("B"), "wp"),
					}.Build(),
					cCheck,
				},
				Stages: []*orchestratorpb.Stage{
					orchestratorpb.Stage_builder{
						Identifier: id.SetWorkplan(id.Stage("A"), "wp"),
					}.Build(),
					orchestratorpb.Stage_builder{
						Identifier: id.SetWorkplan(id.StageWorkNode("B"), "wp"),
					}.Build(),
					orchestratorpb.Stage_builder{
						Identifier: id.SetWorkplan(id.Stage("C"), "wp"),
						Attempts: []*orchestratorpb.Stage_Attempt{
							orchestratorpb.Stage_Attempt_builder{
								Identifier: idspb.StageAttempt_builder{
									Stage: id.SetWorkplan(id.Stage("C"), "wp"),
									Idx:   proto.Int32(1),
								}.Build(),
							}.Build(),
						},
					}.Build(),
				},
			}.Build(),
		)

		assert.Loosely(t,
			nb.Check(id.SetWorkplan(id.Check("A"), "not-present")), should.BeNil)
		assert.That(t,
			nb.Check(id.SetWorkplan(id.Check("C"), "wp")), should.Match(orchestratorpb.Check_builder{
				Identifier: id.SetWorkplan(id.Check("C"), "wp"),
				Options: []*orchestratorpb.Datum{
					orchestratorpb.Datum_builder{
						Identifier: opID,
						Realm:      proto.String("project:realm"),
						Value:      data.Value(&emptypb.Empty{}),
						Version:    &orchestratorpb.Revision{},
					}.Build(),
				},
			}.Build()))

		assert.That(t, nb.Stage(id.SetWorkplan(id.Stage("A"), "wp")), should.Match(orchestratorpb.Stage_builder{
			Identifier: id.SetWorkplan(id.Stage("A"), "wp"),
		}.Build()))

		assert.That(t, nb.Stage(id.SetWorkplan(id.StageWorkNode("B"), "wp")), should.Match(orchestratorpb.Stage_builder{
			Identifier: id.SetWorkplan(id.StageWorkNode("B"), "wp"),
		}.Build()))

		attemptID := idspb.StageAttempt_builder{
			Stage: id.SetWorkplan(id.Stage("C"), "wp"),
			Idx:   proto.Int32(1),
		}.Build()

		assert.That(t, nb.StageAttempt(attemptID), should.Match(orchestratorpb.Stage_Attempt_builder{
			Identifier: attemptID,
		}.Build()))
	})
}
