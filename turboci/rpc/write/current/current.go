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

package current

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/proto/delta"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/rpc/write/attempt"
	"go.chromium.org/luci/turboci/rpc/write/dep"
)

type (
	// Diff modifies a [WriteNodesRequest.CurrentStageWrite].
	//
	// [WriteNodesRequest.CurrentStageWrite]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/orchestrator/v1/write_nodes_request.proto#398
	Diff = delta.Diff[*orchestratorpb.WriteNodesRequest_CurrentStageWrite]

	builder = orchestratorpb.WriteNodesRequest_CurrentStageWrite_builder
)

var template = delta.MakeTemplate[builder](map[string]delta.ApplyMode{
	"state": delta.ModeMaxEnum,
})

// AttemptScheduled returns a Diff that marks this attempt as
// ATTEMPT_STATE_SCHEDULED.
//
// This is meant to be used by Executors which execute Stages asynchronously.
func AttemptScheduled() *Diff {
	return template.New(builder{
		State: attempt.StateScheduled.Enum(),
	})
}

// AttemptRunning returns a Diff that marks this attempt as
// ATTEMPT_STATE_RUNNING, and also sets the process_uid field.
//
// The `process_uid` is meant to disambiguate between all possible
// processes/threads which could potentially execute this Attempt.
//
// An example of a good process_uid would be something like:
//   - the unique hostname || the pid || the threadID
//
// Setting this will allow the orchestrator to detect and reject race conditions
// in executors where multiple processes are assigned the same Attempt and
// attempt to execute it at the same time.
//
// Once an Attempt is marked as Running (and bound to a process_uid), the
// process_uid cannot be modified. For services which don't use persistent
// processes, this should be some value associated with the database key used to
// persist state for the execution of this Attempt.
func AttemptRunning(processUID string) *Diff {
	return template.New(builder{
		State:      attempt.StateRunning.Enum(),
		ProcessUid: &processUID,
	})
}

// AttemptFinished returns a Diff that marks this attempt as either
// ATTEMPT_STATE_COMPLETE or ATTEMPT_STATE_INCOMPLETE (according to the provided
// `complete` bool).
func AttemptFinished(complete bool) *Diff {
	st := attempt.StateIncomplete
	if complete {
		st = attempt.StateComplete
	}
	return template.New(builder{
		State: st.Enum(),
	})
}

// AttemptDetails returns a Diff which sets one or more details for this Stage
// Attempt.
//
// Details are 'write-once', so once a detail of a given type has been written
// to this Attempt, it cannot be overwritten.
func AttemptDetails(details ...proto.Message) *Diff {
	vals, err := data.ValuesErr(details...)
	if err != nil {
		err = fmt.Errorf("current.AttemptDetails: %w", err)
	}
	return template.New(builder{
		Details: vals,
	}, err)
}

// AttemptProgress returns a Diff which appends a Progress message to this
// Attempt.
//
// `msg` must be provided as a message for humans, but use `details` for any
// machine-readable information.
func AttemptProgress(msg string, details ...proto.Message) *Diff {
	vals, err := data.ValuesErr(details...)
	if err != nil {
		err = fmt.Errorf("current.AttemptProgress.details: %w", err)
	}
	prog := orchestratorpb.WriteNodesRequest_StageAttemptProgress_builder{
		Msg:     &msg,
		Details: vals,
	}.Build()
	return template.New(builder{
		Progress: []*orchestratorpb.WriteNodesRequest_StageAttemptProgress{prog},
	}, err)
}

// StageContinuationGroup returns a Diff which sets the continuation_group of
// this Stage.
//
// Note that this is a property of the Stage, not the Stage Attempt.
//
// The continuation_group is a dependency group which delays the Stage state
// from ATTEMPTING to FINAL. The orchestrator will put the Stage into
// AWAITING_GROUP after leaving ATTEMPTING, until the DependencyGroup here
// is resolved (after which the orchestrator will put the Stage into FINAl
// state, potentially unblocking other Stages).
func StageContinuationGroup(diffs ...*dep.Diff) *Diff {
	dg, err := delta.Collect(diffs...)
	if err != nil {
		err = fmt.Errorf("current.ContinuationGroup: %w", err)
	}
	return template.New(builder{
		ContinuationGroup: dg,
	}, err)
}

// AttemptExecutionPolicy returns a Diff which sets the AttemptExecutionPolicy
// for the current Attempt.
//
// Can only be set when transitioning from PENDING to SCHEDULED or RUNNING.
func AttemptExecutionPolicy(diffs ...*attempt.PolicyDiff) *Diff {
	pol, err := delta.Collect(diffs...)
	return template.New(builder{
		AttemptExecutionPolicy: pol,
	}, err)
}
