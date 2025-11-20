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

package check

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/proto/delta"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write/dep"
)

type (
	// TargetDiff modifies an [Edge.Check].
	//
	// This is used with [Edge].
	//
	// [Edge.Check]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/orchestrator/v1/edge.proto#39
	TargetDiff = delta.Diff[*orchestratorpb.Edge_Check]

	targetMsgBuilder = orchestratorpb.Edge_Check_builder
)

var targetTemplate = delta.MakeTemplate[targetMsgBuilder](map[string]delta.ApplyMode{
	"identifier": delta.ModeMerge,
})

// TargetInWorkplan returns a Diff which sets the WorkPlan of this Edge.Check's
// identifier.
//
// NOTE: As of 2026Q1, cross-WorkPlan dependencies are not supported.
func TargetInWorkplan(workplanID string) *TargetDiff {
	return targetTemplate.New(targetMsgBuilder{
		Identifier: idspb.Check_builder{
			WorkPlan: idspb.WorkPlan_builder{
				Id: &workplanID,
			}.Build(),
		}.Build(),
	})
}

// TargetCondition returns a Diff which sets the condition of the Edge.Check.
//
// `expression` is optional; If provided more than once, the expressions will be
// joined together with `&&`.
//
// NOTE: As of 2026Q1, the only supported expression is blank or "true".
func TargetCondition(onState State, expression ...string) *TargetDiff {
	cond := &orchestratorpb.Edge_Check_Condition{}
	if onState != StateUnknown {
		cond.SetOnState(onState)
	}
	if len(expression) == 1 {
		cond.SetExpression(expression[0])
	} else if len(expression) > 1 {
		cond.SetExpression(
			fmt.Sprintf("(%s)", strings.Join(expression, ")&&(")))
	}
	return targetTemplate.New(targetMsgBuilder{
		Condition: cond,
	})
}

// Edge returns a Diff which adds a Check-type Edge to a DependencyGroup.
func Edge(target string, diffs ...*TargetDiff) *dep.Diff {
	var errs []error
	cid, err := id.CheckErr(target)
	if err != nil {
		errs = append(errs, fmt.Errorf("check.Edge: %w", err))
	}
	edge := targetMsgBuilder{
		Identifier: cid,
	}.Build()
	for _, diff := range diffs {
		if err := diff.Apply(edge); err != nil {
			errs = append(errs, fmt.Errorf("check.Edge: %w", err))
		}
	}
	return dep.Template.New(dep.Builder{
		Edges: []*orchestratorpb.Edge{orchestratorpb.Edge_builder{
			Check: edge,
		}.Build()},
	}, errs...)
}
