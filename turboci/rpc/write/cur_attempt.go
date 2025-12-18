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

package write

import (
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
)

// GetCurrentStage returns a helper for manipulating the `current_stage` of the
// given request.
//
// If the request does not already have a value for `current_stage`, it will be
// populated with an empty message as a result of this function.
func (req Request) GetCurrentStage() CurrentStageWrite {
	csw := req.Msg.GetCurrentStage()
	if csw == nil {
		csw = &orchestratorpb.WriteNodesRequest_CurrentStageWrite{}
		req.Msg.SetCurrentStage(csw)
	}
	return CurrentStageWrite{csw}
}

// CurrentStageWrite is a helper to manipulate a CurrentStageWrite message.
//
// As produced by this library, this never contains a nil pointer.
type CurrentStageWrite struct {
	Msg *orchestratorpb.WriteNodesRequest_CurrentStageWrite
}

// GetCurrentAttempt returns a helper for manipulating the `current_attempt`
// of the given request.
func (req Request) GetCurrentAttempt() CurrentAttemptWrite {
	caw := req.Msg.GetCurrentAttempt()
	if caw == nil {
		caw = &orchestratorpb.WriteNodesRequest_CurrentAttemptWrite{}
		req.Msg.SetCurrentAttempt(caw)
	}
	return CurrentAttemptWrite{caw}
}

// CurrentAttemptWrite is a helper to manipulate a CurrentAttemptWrite message.
//
// As produced by this library, this never contains a nil pointer.
type CurrentAttemptWrite struct {
	Msg *orchestratorpb.WriteNodesRequest_CurrentAttemptWrite
}

// AddProgress adds, and returns, a new progress message (with protobuf details)
// to the current stage attempt.
//
// This only returns an error if there is a problem converting `details` to
// Value messages.
func (cswca CurrentAttemptWrite) AddProgress(message string, details ...proto.Message) (*orchestratorpb.WriteNodesRequest_StageAttemptProgress, error) {
	vals, err := data.ValuesErr(details...)
	if err != nil {
		return nil, err
	}
	ret := orchestratorpb.WriteNodesRequest_StageAttemptProgress_builder{
		Message: &message,
		Details: vals,
	}.Build()
	cswca.Msg.SetProgress(append(cswca.Msg.GetProgress(), ret))
	return ret, nil
}

// AddDetails adds the given proto messages as details to the current stage
// attempt.
func (cswca CurrentAttemptWrite) AddDetails(details ...proto.Message) ([]*orchestratorpb.Value, error) {
	vals, err := data.ValuesErr(details...)
	if err != nil {
		return nil, err
	}
	cswca.Msg.SetDetails(append(cswca.Msg.GetDetails(), vals...))
	return vals, nil
}

// StateTransition is a helper to manipulate the state_transition portion of a
// CurrentAttemptWrite message.
//
// As produced by this library, this never contains a nil pointer.
type StateTransition struct {
	Msg *orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition
}

// GetStateTransition returns a helper for manipulating the `state_transition`
// portion of the CurrentAttemptWrite.
func (cswca CurrentAttemptWrite) GetStateTransition() StateTransition {
	sa := cswca.Msg.GetStateTransition()
	if sa == nil {
		sa = &orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition{}
		cswca.Msg.SetStateTransition(sa)
	}
	return StateTransition{sa}
}

// SetThrottled sets this StateTransition to "THROTTLED".
func (sa StateTransition) SetThrottled(until time.Time) {
	sa.Msg.SetThrottled(orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition_Throttled_builder{
		Until: timestamppb.New(until),
	}.Build())
}

// SetScheduled sets this StateTransition to "SCHEDULED".
func (sa StateTransition) SetScheduled(executionPolicy *orchestratorpb.StageAttemptExecutionPolicy) {
	sa.Msg.SetScheduled(orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition_Scheduled_builder{
		AttemptExecutionPolicy: executionPolicy,
	}.Build())
}

// SetTearingDown sets this StateTransition to "TEARING_DOWN".
func (sa StateTransition) SetTearingDown() {
	sa.Msg.SetTearingDown(&orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition_TearingDown{})
}

// SetComplete sets this StateTransition to "COMPLETE".
func (sa StateTransition) SetComplete() {
	sa.Msg.SetComplete(&orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition_Complete{})
}

// SetIncomplete sets this StateTransition to "INCOMPLETE".
func (sa StateTransition) SetIncomplete(blockNewAttempts bool) {
	var bna *bool
	if blockNewAttempts {
		bna = &blockNewAttempts
	}
	sa.Msg.SetIncomplete(orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition_Incomplete_builder{
		BlockNewAttempts: bna,
	}.Build())
}

// SetRunning sets this StateTransition to "RUNNING".
func (sa StateTransition) SetRunning(processUID string, executionPolicy *orchestratorpb.StageAttemptExecutionPolicy) {
	sa.Msg.SetRunning(orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition_Running_builder{
		ProcessUid:             &processUID,
		AttemptExecutionPolicy: executionPolicy,
	}.Build())
}
