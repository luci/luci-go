// Copyright 2022 The LUCI Authors.
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

package execute

import (
	"fmt"
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUpdateAttempts(t *testing.T) {
	Convey("UpdateAttempts", t, func() {
		execState := newExecStateBuilder().withDefinition(makeDefinition("critical", true)).appendAttempt(0, 101).state
		Convey("update", func() {
			Convey("tryjob status, result", func() {
				updateAttempts(execState, map[common.TryjobID]*tryjob.Tryjob{
					common.TryjobID(101): {ID: 101, Status: tryjob.Status_ENDED, Result: &tryjob.Result{Status: tryjob.Result_TIMEOUT}},
				})
				So(execState.Executions[0].Attempts[0].Status, ShouldEqual, tryjob.Status_ENDED)
				updateAttempts(execState, map[common.TryjobID]*tryjob.Tryjob{
					common.TryjobID(101): {ID: 101, Status: tryjob.Status_ENDED, Result: &tryjob.Result{Status: tryjob.Result_SUCCEEDED}},
				})
				So(execState.Executions[0].Attempts[0].Result.Status, ShouldEqual, tryjob.Result_SUCCEEDED)
			})
			Convey("ignore previous attempts", func() {
				updateAttempts(execState, map[common.TryjobID]*tryjob.Tryjob{
					common.TryjobID(101): {ID: 101, Status: tryjob.Status_ENDED, Result: &tryjob.Result{Status: tryjob.Result_SUCCEEDED}},
				})
				execState = newExecStateBuilder(execState).appendAttempt(0, 102).state
				prevAttemptStatus := execState.Executions[0].Attempts[0].Result.Status
				updateAttempts(execState, map[common.TryjobID]*tryjob.Tryjob{
					common.TryjobID(101): {ID: 101, Status: tryjob.Status_ENDED, Result: &tryjob.Result{Status: tryjob.Result_FAILED_PERMANENTLY}},
				})
				So(execState.Executions[0].Attempts[0].Result.Status, ShouldEqual, prevAttemptStatus)
			})
			Convey("ignore no longer required tryjobs", func() {
				originalState := proto.Clone(execState).(*tryjob.ExecutionState)
				updateAttempts(execState, map[common.TryjobID]*tryjob.Tryjob{
					common.TryjobID(201): {ID: 201, Status: tryjob.Status_ENDED, Result: &tryjob.Result{Status: tryjob.Result_FAILED_PERMANENTLY}},
				})
				So(execState, ShouldResembleProto, originalState)
			})
		})
	})
}

type execStateBuilder struct {
	state *tryjob.ExecutionState
}

func newExecStateBuilder(original ...*tryjob.ExecutionState) *execStateBuilder {
	switch len(original) {
	case 0:
		return &execStateBuilder{
			state: &tryjob.ExecutionState{Requirement: &tryjob.Requirement{}},
		}
	case 1:
		return &execStateBuilder{
			state: original[0],
		}
	default:
		panic(fmt.Errorf("more than one original execState provided"))
	}
}

func (esb *execStateBuilder) withDefinition(def *tryjob.Definition) *execStateBuilder {
	esb.state.Requirement.Definitions = append(esb.state.Requirement.Definitions, def)
	esb.state.Executions = append(esb.state.Executions, &tryjob.ExecutionState_Execution{})
	return esb
}

func (esb *execStateBuilder) appendAttempt(executionIndex int, id int64) *execStateBuilder {
	esb.state.Executions[executionIndex].Attempts = append(esb.state.Executions[executionIndex].Attempts, &tryjob.ExecutionState_Execution_Attempt{
		TryjobId: id,
		Status:   tryjob.Status_PENDING,
	})
	return esb
}

func makeDefinition(builder string, critical bool) *tryjob.Definition {
	return &tryjob.Definition{
		Backend: &tryjob.Definition_Buildbucket_{
			Buildbucket: &tryjob.Definition_Buildbucket{
				Host: "test.com",
				Builder: &buildbucketpb.BuilderID{
					Project: "test",
					Bucket:  "bucket",
					Builder: builder,
				},
			},
		},
		Critical: critical,
	}
}
