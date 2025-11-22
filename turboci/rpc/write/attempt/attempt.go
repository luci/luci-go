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

// Package attempt has types and functions for building writable messages
// for a TurboCI Stage Attempt, notably [StageAttemptExecutionPolicy].
//
// Used in conjunction with [go.chromium.org/luci/turboci].
//
// [StageAttemptExecutionPolicy]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/orchestrator/v1/stage_attempt_execution_policy.proto#14
package attempt

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/proto/delta"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

type (
	// PolicyDiff modifies a [StageAttemptExecutionPolicy].
	//
	// [StageAttemptExecutionPolicy]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/orchestrator/v1/stage_attempt_execution_policy.proto#14
	PolicyDiff = delta.Diff[*orchestratorpb.StageAttemptExecutionPolicy]

	builder = orchestratorpb.StageAttemptExecutionPolicy_builder
)

var template = delta.MakeTemplate[builder](map[string]delta.ApplyMode{
	"heartbeat": delta.ModeMerge,
	"timeout":   delta.ModeMerge,
})

// HeartbeatScheduled returns a Diff which sets the `heartbeat.scheduled` of the
// StageAttemptExecutionPolicy.
func HeartbeatScheduled(dur time.Duration) *PolicyDiff {
	durPB, err := maybeDuration("Scheduled", dur)
	return template.New(builder{
		Heartbeat: orchestratorpb.StageAttemptExecutionPolicy_Heartbeat_builder{
			Scheduled: durPB,
		}.Build(),
	}, err)
}

// HeartbeatRunning returns a Diff which sets the `heartbeat.running` of the
// StageAttemptExecutionPolicy.
func HeartbeatRunning(dur time.Duration) *PolicyDiff {
	durPB, err := maybeDuration("Running", dur)
	return template.New(builder{
		Heartbeat: orchestratorpb.StageAttemptExecutionPolicy_Heartbeat_builder{
			Running: durPB,
		}.Build(),
	}, err)
}

// HeartbeatTearingDown returns a Diff which sets the `heartbeat.tearing_down`
// of the StageAttemptExecutionPolicy.
func HeartbeatTearingDown(dur time.Duration) *PolicyDiff {
	durPB, err := maybeDuration("TearingDown", dur)
	return template.New(builder{
		Heartbeat: orchestratorpb.StageAttemptExecutionPolicy_Heartbeat_builder{
			TearingDown: durPB,
		}.Build(),
	}, err)
}

func maybeDuration(name string, dur time.Duration) (*durationpb.Duration, error) {
	if dur == 0 {
		return nil, nil
	}
	if dur < 0 {
		return nil, fmt.Errorf("%s: %v: must be positive", name, dur)
	}
	return durationpb.New(dur), nil
}

// TimeoutPendingThrottled returns a Diff which sets the
// `timeout.pending_throttled` of the StageAttemptExecutionPolicy.
func TimeoutPendingThrottled(dur time.Duration) *PolicyDiff {
	durPB, err := maybeDuration("PendingThrottled", dur)
	return template.New(builder{
		Timeout: orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
			PendingThrottled: durPB,
		}.Build(),
	}, err)
}

// TimeoutScheduled returns a Diff which sets the `timeout.scheduled` of the
// StageAttemptExecutionPolicy.
func TimeoutScheduled(dur time.Duration) *PolicyDiff {
	durPB, err := maybeDuration("Scheduled", dur)
	return template.New(builder{
		Timeout: orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
			Scheduled: durPB,
		}.Build(),
	}, err)
}

// TimeoutRunning returns a Diff which sets the `timeout.scheduled` of the
// StageAttemptExecutionPolicy.
func TimeoutRunning(dur time.Duration) *PolicyDiff {
	durPB, err := maybeDuration("Running", dur)
	return template.New(builder{
		Timeout: orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
			Running: durPB,
		}.Build(),
	}, err)
}

// TimeoutTearingDown returns a Diff which sets the `timeout.scheduled` of the
// StageAttemptExecutionPolicy.
func TimeoutTearingDown(dur time.Duration) *PolicyDiff {
	durPB, err := maybeDuration("TearingDown", dur)
	return template.New(builder{
		Timeout: orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
			TearingDown: durPB,
		}.Build(),
	}, err)
}
