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

import orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

type State = orchestratorpb.StageState

const (
	StateUnknown       State = orchestratorpb.StageState_STAGE_STATE_UNKNOWN
	StatePlanned       State = orchestratorpb.StageState_STAGE_STATE_PLANNED
	StateAttempting    State = orchestratorpb.StageState_STAGE_STATE_ATTEMPTING
	StateAwaitingGroup State = orchestratorpb.StageState_STAGE_STATE_AWAITING_GROUP
	StateFinal         State = orchestratorpb.StageState_STAGE_STATE_FINAL
)

type AttemptState = orchestratorpb.StageAttemptState

const (
	AttemptStateUnknown       AttemptState = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_UNKNOWN
	AttemptStatePending       AttemptState = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_PENDING
	AttemptStateScheduled     AttemptState = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_SCHEDULED
	AttemptStateRunning       AttemptState = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING
	AttemptStateCancelling    AttemptState = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_CANCELLING
	AttemptStateTearingDown   AttemptState = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_TEARING_DOWN
	AttemptStateComplete      AttemptState = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_COMPLETE
	AttemptStateIncomplete    AttemptState = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_INCOMPLETE
	AttemptStateAwaitingRetry AttemptState = orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_AWAITING_RETRY
)
