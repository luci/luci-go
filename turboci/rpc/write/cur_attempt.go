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
	"google.golang.org/protobuf/proto"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
)

// CurrentStageWrite is a helper to manipulate a CurrentStageWrite message.
//
// As produced by this library, this never contains a nil pointer.
type CurrentStageWrite struct {
	Msg *orchestratorpb.WriteNodesRequest_CurrentStageWrite
}

// AddProgress adds, and returns, a new progress message (with protobuf details)
// to the current stage attempt.
//
// This only returns an error if there is a problem converting `details` to
// Value messages.
func (csw CurrentStageWrite) AddProgress(msg string, details ...proto.Message) (*orchestratorpb.WriteNodesRequest_StageAttemptProgress, error) {
	vals, err := data.ValuesErr(details...)
	if err != nil {
		return nil, err
	}
	ret := orchestratorpb.WriteNodesRequest_StageAttemptProgress_builder{
		Msg:     &msg,
		Details: vals,
	}.Build()
	csw.Msg.SetProgress(append(csw.Msg.GetProgress(), ret))
	return ret, nil
}

// AddDetails adds the given proto messages as details to the current stage
// attempt.
func (csw CurrentStageWrite) AddDetails(details ...proto.Message) ([]*orchestratorpb.Value, error) {
	vals, err := data.ValuesErr(details...)
	if err != nil {
		return nil, err
	}
	csw.Msg.SetDetails(append(csw.Msg.GetDetails(), vals...))
	return vals, nil
}

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
