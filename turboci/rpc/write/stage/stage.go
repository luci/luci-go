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

// Package stage has types and functions for building writable messages
// for a TurboCI Stage, notably [WriteNodesRequest.StageWrite].
//
// Used in conjunction with [go.chromium.org/luci/turboci].
//
// [WriteNodesRequest.StageWrite]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/orchestrator/v1/write_nodes_request.proto#214
package stage

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/proto/delta"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write/attempt"
	"go.chromium.org/luci/turboci/rpc/write/check"
	"go.chromium.org/luci/turboci/rpc/write/dep"
)

type (
	// Diff modifies a [WriteNodesRequest.StageWrite].
	//
	// [WriteNodesRequest.StageWrite]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/orchestrator/v1/write_nodes_request.proto#214
	Diff = delta.Diff[*orchestratorpb.WriteNodesRequest_StageWrite]

	builder = orchestratorpb.WriteNodesRequest_StageWrite_builder
)

var template = delta.MakeTemplate[builder](map[string]delta.ApplyMode{
	"identifier":                       delta.ModeMerge,
	"requested_stage_execution_policy": delta.ModeMerge,
})

// Realm is used to set the realm for a Stage.
//
// If this is used for an existing Stage, it must exactly match the
// realm of the existing Stage.
func Realm(realm string) *Diff {
	return template.New(builder{
		Realm: &realm,
	})
}

// InWorkplan sets the WorkPlan of this StageWrite's identifier.
//
// NOTE: As of 2026Q1, cross-WorkPlan writes are not supported.
func InWorkplan(workplanID string) *Diff {
	return template.New(builder{
		Identifier: idspb.Stage_builder{
			WorkPlan: idspb.WorkPlan_builder{
				Id: &workplanID,
			}.Build(),
		}.Build(),
	})
}

// IsWorknode marks this Stage as being of type `WorkNode`.
//
// Normally this is detected automatically based on the `args` by
// [go.chromium.org/luci/turboci/rpc/write.NewStage], but this is provided as
// a way to override this determination.
func IsWorknode(value bool) *Diff {
	return template.New(builder{
		Identifier: idspb.Stage_builder{
			IsWorknode: &value,
		}.Build(),
	})
}

// Deps returns a Diff which sets the Dependencies of the StageWrite.
func Deps(diffs ...*dep.Diff) *Diff {
	dg, err := delta.Collect(diffs...)
	if err != nil {
		err = fmt.Errorf("stage.Deps: %w", err)
	}
	return template.New(builder{
		Dependencies: dg,
	}, err)
}

type assignment = orchestratorpb.Stage_Assignment

func assign(name string, goalState orchestratorpb.CheckState, checks []string) ([]*assignment, []error) {
	assignments := make([]*orchestratorpb.Stage_Assignment, 0, len(checks))
	var errs []error
	for _, checkID := range checks {
		cid, err := id.CheckErr(checkID)
		if err != nil {
			errs = append(errs, fmt.Errorf("stage.%s: %w", name, err))
		} else {
			assignments = append(assignments, orchestratorpb.Stage_Assignment_builder{
				GoalState: goalState.Enum(),
				Target:    cid,
			}.Build())
		}
	}
	return assignments, errs
}

// ShouldPlan returns a Diff which adds an [Assignment] to this Stage.
//
// All the checks indicated here by id will be assigned to this Stage with
// a `goal_state` of CHECK_STATE_PLANNING.
//
// [Assignment]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/orchestrator/v1/stage.proto#271
func ShouldPlan(checks ...string) *Diff {
	assignments, errs := assign("ShouldPlan", check.StatePlanning, checks)
	return template.New(builder{
		Assignments: assignments,
	}, errs...)
}

// ShouldFinalize returns a Diff which adds an [Assignment] to this Stage.
//
// All the checks indicated here by id will be assigned to this Stage with
// a `goal_state` of CHECK_STATE_FINAL.
//
// [Assignment]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/orchestrator/v1/stage.proto#271
func ShouldFinalize(checks ...string) *Diff {
	assignments, errs := assign("ShouldFinalize", check.StateFinal, checks)
	return template.New(builder{
		Assignments: assignments,
	}, errs...)
}

// MaxRetries returns a Diff which sets
// requested_stage_execution_policy.retry.max_retries to `maxRetries`.
func MaxRetries(maxRetries int) *Diff {
	return template.New(builder{
		RequestedStageExecutionPolicy: orchestratorpb.StageExecutionPolicy_builder{
			Retry: orchestratorpb.StageExecutionPolicy_Retry_builder{
				MaxRetries: proto.Int32(int32(maxRetries)),
			}.Build(),
		}.Build(),
	})
}

// Timeout returns a Diff which sets
// requested_stage_execution_policy.stage_timeout and ...stage_timeout_mode to
// `dur` and `mode` respectively.
func Timeout(dur time.Duration, mode TimeoutMode) *Diff {
	return template.New(builder{
		RequestedStageExecutionPolicy: orchestratorpb.StageExecutionPolicy_builder{
			StageTimeout:     durationpb.New(dur),
			StageTimeoutMode: mode.Enum(),
		}.Build(),
	})
}

// AttemptExecutionPolicy sets the requested attempt policy template for this
// Stage.
//
// The Executor for the Stage has ultimate authority for the policy on
// a per-Attempt basis, but it can take this template into account.
func AttemptExecutionPolicy(diffs ...*attempt.PolicyDiff) *Diff {
	pol, err := delta.Collect(diffs...)
	return template.New(builder{
		RequestedStageExecutionPolicy: orchestratorpb.StageExecutionPolicy_builder{
			AttemptExecutionPolicyTemplate: pol,
		}.Build(),
	}, err)
}
