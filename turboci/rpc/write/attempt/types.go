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

package attempt

import orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

// State is a shorthand equivalent to orchestratorpb.StageAttemptState.
type State = orchestratorpb.StageAttemptState

// These are shorthand equivalents to
// orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_*.
const (
	StateUnknown       State = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_UNKNOWN
	StatePending       State = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_PENDING
	StateScheduled     State = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_SCHEDULED
	StateRunning       State = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING
	StateCancelling    State = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_CANCELLING
	StateTearingDown   State = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_TEARING_DOWN
	StateComplete      State = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_COMPLETE
	StateIncomplete    State = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_INCOMPLETE
	StateAwaitingRetry State = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_AWAITING_RETRY
)
