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

package stage

import (
	"fmt"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// State is a shorthand equivalent to [orchestratorpb.StageState].
type State = orchestratorpb.StageState

// These are shorthand equivalents to orchestratorpb.StageState_STAGE_STATE_*.
const (
	StateUnknown       State = orchestratorpb.StageState_STAGE_STATE_UNKNOWN
	StatePlanned       State = orchestratorpb.StageState_STAGE_STATE_PLANNED
	StateAttempting    State = orchestratorpb.StageState_STAGE_STATE_ATTEMPTING
	StateAwaitingGroup State = orchestratorpb.StageState_STAGE_STATE_AWAITING_GROUP
	StateFinal         State = orchestratorpb.StageState_STAGE_STATE_FINAL
)

// TimeoutMode is a shorthand equivalent to
// [orchestratorpb.StageExecutionPolicy_StageTimeoutMode].
type TimeoutMode = orchestratorpb.StageExecutionPolicy_StageTimeoutMode

// These are shorthand equivalents to
// orchestratorpb.StageExecutionPolicy_STAGE_TIMEOUT_MODE_*.
const (
	TimeoutModeUnknown                TimeoutMode = orchestratorpb.StageExecutionPolicy_STAGE_TIMEOUT_MODE_UNKNOWN
	TimeoutModeFinishCurrentAttempt   TimeoutMode = orchestratorpb.StageExecutionPolicy_STAGE_TIMEOUT_MODE_FINISH_CURRENT_ATTEMPT
	TimeoutModeBlockMaxExecutionRetry TimeoutMode = orchestratorpb.StageExecutionPolicy_STAGE_TIMEOUT_MODE_BLOCK_MAX_EXECUTION_RETRY
	TimeoutModeHybrid                 TimeoutMode = orchestratorpb.StageExecutionPolicy_STAGE_TIMEOUT_MODE_HYBRID
)

func applyTimeoutMode(t TimeoutMode, ep *orchestratorpb.StageExecutionPolicy) error {
	switch t {
	case TimeoutModeUnknown,
		TimeoutModeFinishCurrentAttempt,
		TimeoutModeBlockMaxExecutionRetry,
		TimeoutModeHybrid:
	default:
		return fmt.Errorf("unknown stage.TimeoutMode: %q", t)
	}

	if t == TimeoutModeUnknown {
		ep.ClearStageTimeoutMode()
	} else {
		ep.SetStageTimeoutMode(t)
	}
	return nil
}
