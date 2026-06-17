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

package dep_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/check"
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write/dep"
	"go.chromium.org/luci/turboci/stage"
)

func TestGroup(t *testing.T) {
	t.Parallel()

	grp := dep.MustGroup(
		id.Check("check"),
		id.Stage("stage"),
		dep.ConditionalCheck(id.Check("other"), check.StatePlanned, `bogus == "true"`),
		dep.ConditionalStage(id.Stage("stage other"), stage.StateAwaitingGroup, `true`, `false`),
		dep.MustGroup(
			id.Check("a"),
			id.Check("b"),
		),
		dep.Threshold(2),
	)

	assert.That(t, grp, should.Match(orchestratorpb.WriteNodesRequest_DependencyGroup_builder{
		Edges: []*orchestratorpb.Edge{
			orchestratorpb.Edge_builder{
				Check: orchestratorpb.Edge_Check_builder{
					Identifier: id.Check("check"),
				}.Build(),
			}.Build(),
			orchestratorpb.Edge_builder{
				Stage: orchestratorpb.Edge_Stage_builder{
					Identifier: id.Stage("stage"),
				}.Build(),
			}.Build(),
			orchestratorpb.Edge_builder{
				Check: orchestratorpb.Edge_Check_builder{
					Identifier: id.Check("other"),
					Condition: orchestratorpb.Edge_Check_Condition_builder{
						OnState:    check.StatePlanned.Enum(),
						Expression: proto.String(`bogus == "true"`),
					}.Build(),
				}.Build(),
			}.Build(),
			orchestratorpb.Edge_builder{
				Stage: orchestratorpb.Edge_Stage_builder{
					Identifier: id.Stage("stage other"),
					Condition: orchestratorpb.Edge_Stage_Condition_builder{
						OnState:    stage.StateAwaitingGroup.Enum(),
						Expression: proto.String(`(true)&&(false)`),
					}.Build(),
				}.Build(),
			}.Build(),
		},
		Groups: []*orchestratorpb.WriteNodesRequest_DependencyGroup{
			orchestratorpb.WriteNodesRequest_DependencyGroup_builder{
				Edges: []*orchestratorpb.Edge{
					orchestratorpb.Edge_builder{
						Check: orchestratorpb.Edge_Check_builder{
							Identifier: id.Check("a"),
						}.Build(),
					}.Build(),
					orchestratorpb.Edge_builder{
						Check: orchestratorpb.Edge_Check_builder{
							Identifier: id.Check("b"),
						}.Build(),
					}.Build(),
				},
			}.Build(),
		},
		Threshold: proto.Int32(2),
	}.Build()))
}
