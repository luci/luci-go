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

// Package dep contains helpers for building TurboCI DependencyGroup
// messages for use in a WriteNodesRequest.
package dep

import (
	"errors"
	"fmt"
	"strings"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/check"
	"go.chromium.org/luci/turboci/stage"
)

func makeConditionExp(expressions []string) *string {
	if len(expressions) == 0 {
		return nil
	}
	if len(expressions) == 1 {
		return &expressions[0]
	}
	ret := fmt.Sprintf("(%s)", strings.Join(expressions, ")&&("))
	return &ret
}

// ConditionalCheck returns an Edge for a Check with the condition described by
// `onState` and `cond`.
//
// `cond` should be a CEL expression; if more than one value is passed for
// `cond`, they are AND'd together like: `(cond[0)&&(cond[1])...`.
func ConditionalCheck(id *idspb.Check, onState check.State, cond ...string) *orchestratorpb.Edge {
	return orchestratorpb.Edge_builder{
		Check: orchestratorpb.Edge_Check_builder{
			Identifier: id,
			Condition: orchestratorpb.Edge_Check_Condition_builder{
				OnState:    &onState,
				Expression: makeConditionExp(cond),
			}.Build(),
		}.Build(),
	}.Build()
}

// ConditionalStage returns an Edge for a Stage with the condition described by
// `onState` and `cond`.
//
// `cond` should be a CEL expression; if more than one value is passed for
// `cond`, they are AND'd together like: `(cond[0)&&(cond[1])...`.
func ConditionalStage(id *idspb.Stage, onState stage.State, cond ...string) *orchestratorpb.Edge {
	return orchestratorpb.Edge_builder{
		Stage: orchestratorpb.Edge_Stage_builder{
			Identifier: id,
			Condition: orchestratorpb.Edge_Stage_Condition_builder{
				OnState:    &onState,
				Expression: makeConditionExp(cond),
			}.Build(),
		}.Build(),
	}.Build()
}

// Threshold is passed to [Group] to set the Threshold of the returned Group.
type Threshold int32

// Group accepts any of the following in any order:
//
//   - [*idspb.Stage]: Adds a Stage edge with the default condition (onState
//     FINAL, cond == "true").
//   - [*idspb.Check]: Adds a Check edge with the default condition (onState
//     FINAL, cond == "true").
//   - [*orchestratorpb.Edge]: Added verbatim to edges.
//   - [*orchestratorpb.WriteNodesRequest_DependencyGroup]: Added verbatim to
//     groups.
//   - [Threshold]: Sets the threshold (if supplied multiple times,
//     the last one wins).
//
// This will return the assembled DependencyGroup, plus an error if any `items`
// were of unknown types - see [MustGroup] if you statically know that you
// haven't passed bogus types to this :).
func Group(items ...any) (*orchestratorpb.WriteNodesRequest_DependencyGroup, error) {
	edges := make([]*orchestratorpb.Edge, 0, len(items))
	groups := make([]*orchestratorpb.WriteNodesRequest_DependencyGroup, 0, len(items))
	var threshold int32

	var errs []error
	for i, item := range items {
		switch x := item.(type) {
		case *idspb.Check:
			edges = append(edges, orchestratorpb.Edge_builder{
				Check: orchestratorpb.Edge_Check_builder{
					Identifier: x,
				}.Build(),
			}.Build())

		case *idspb.Stage:
			edges = append(edges, orchestratorpb.Edge_builder{
				Stage: orchestratorpb.Edge_Stage_builder{
					Identifier: x,
				}.Build(),
			}.Build())

		case *orchestratorpb.Edge:
			edges = append(edges, x)

		case *orchestratorpb.WriteNodesRequest_DependencyGroup:
			groups = append(groups, x)

		case Threshold:
			threshold = int32(x)

		default:
			errs = append(errs, fmt.Errorf("dep.Group: items[%d]: unknown type %T", i, x))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	ret := orchestratorpb.WriteNodesRequest_DependencyGroup_builder{
		Edges:  edges,
		Groups: groups,
	}.Build()
	if threshold != 0 {
		ret.SetThreshold(threshold)
	}
	return ret, nil
}

// MustGroup is the same as [Group] except it panics if any `items` are of
// unknown types.
func MustGroup(items ...any) *orchestratorpb.WriteNodesRequest_DependencyGroup {
	ret, err := Group(items...)
	if err != nil {
		panic(err)
	}
	return ret
}
