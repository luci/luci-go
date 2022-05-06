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
	"context"
	"testing"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestNeedsRetry(t *testing.T) {
	Convey("needsRetry", t, func() {
		ctx := context.Background()
		execState := newExecStateBuilder().
			appendDefinition(makeDefinition("critical", true)).appendAttempt(0, 101).
			appendDefinition(makeDefinition("nonCritical", false)).appendAttempt(1, 201).state
		criticalExecution := execState.Executions[0]
		criticalDefinition := execState.Requirement.Definitions[0]
		nonCriticalExecution := execState.Executions[1]
		nonCriticalDefinition := execState.Requirement.Definitions[1]

		Convey("retry not needed", func() {
			Convey("non-critical", func() {
				nonCriticalExecution.Attempts[0].Result = &tryjob.Result{Status: tryjob.Result_FAILED_PERMANENTLY}
				nonCriticalExecution.Attempts[0].Status = tryjob.Status_ENDED
				r := needsRetry(ctx, nonCriticalDefinition, nonCriticalExecution)
				So(r, ShouldEqual, retryNotNeeded)
			})
			Convey("in progress", func() {
				criticalExecution.Attempts[0].Status = tryjob.Status_PENDING
				r := needsRetry(ctx, criticalDefinition, criticalExecution)
				So(r, ShouldEqual, retryNotNeeded)
			})
			Convey("succeeded", func() {
				criticalExecution.Attempts[0].Result = &tryjob.Result{Status: tryjob.Result_SUCCEEDED}
				criticalExecution.Attempts[0].Status = tryjob.Status_ENDED
				r := needsRetry(ctx, criticalDefinition, criticalExecution)
				So(r, ShouldEqual, retryNotNeeded)
			})
		})
		Convey("reused tryjob failed", func() {
			criticalExecution.Attempts[0].Reused = true
			Convey("returns correct value", func() {
				criticalExecution.Attempts[0].Result = &tryjob.Result{Status: tryjob.Result_FAILED_PERMANENTLY}
				criticalExecution.Attempts[0].Status = tryjob.Status_ENDED
				Convey("with no recipe output", func() {
					criticalExecution.Attempts[0].Result.Output = nil
				})
				Convey("with retry denied property", func() {
					criticalExecution.Attempts[0].Result.Output = &recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED}
				})
				r := needsRetry(ctx, criticalDefinition, criticalExecution)
				So(r, ShouldEqual, retryRequiredIgnoreQuota)
			})
		})
		Convey("not reused", func() {
			Convey("retry denied", func() {
				criticalExecution.Attempts[0].Result = &tryjob.Result{Status: tryjob.Result_FAILED_PERMANENTLY, Output: &recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED}}
				criticalExecution.Attempts[0].Status = tryjob.Status_ENDED
				r := needsRetry(ctx, criticalDefinition, criticalExecution)
				So(r, ShouldEqual, retryDenied)
			})
			Convey("retry required", func() {
				criticalExecution.Attempts[0].Result = &tryjob.Result{Status: tryjob.Result_FAILED_PERMANENTLY}
				criticalExecution.Attempts[0].Status = tryjob.Status_ENDED
				r := needsRetry(ctx, criticalDefinition, criticalExecution)
				So(r, ShouldEqual, retryRequired)

			})
		})
	})
}

func TestComputeRetries(t *testing.T) {
	Convey("ComputeRetries", t, func() {
		ctx := context.Background()
		execState := newExecStateBuilder().
			appendDefinition(makeDefinition("builder-one", true)).appendAttempt(0, 101).
			appendDefinition(makeDefinition("builder-two", true)).appendAttempt(1, 201).
			appendDefinition(makeDefinition("builder-three", false)).appendAttempt(2, 301).
			state

		Convey("with no retry config", func() {
			Convey("with no tryjobs", func() {
				So(computeRetries(ctx, execState, map[common.TryjobID]*tryjob.Tryjob{}), ShouldHaveLength, 0)
				So(execState.Status, ShouldNotEqual, tryjob.ExecutionState_FAILED)
			})
			Convey("with failed experimental", func() {
				failedTryjobs := map[common.TryjobID]*tryjob.Tryjob{
					301: {
						ID:         301,
						ExternalID: tryjob.MustBuildbucketID("host.com", 301),
						Result: &tryjob.Result{
							Status: tryjob.Result_FAILED_TRANSIENTLY,
						},
						Status: tryjob.Status_ENDED,
					},
				}
				updateAttempts(execState, failedTryjobs)
				retries := computeRetries(ctx, execState, failedTryjobs)
				So(retries, ShouldHaveLength, 0)
				So(execState.Status, ShouldNotEqual, tryjob.ExecutionState_FAILED)
			})
			Convey("with failed critical", func() {
				failedTryjobs := map[common.TryjobID]*tryjob.Tryjob{
					101: {
						ID:         101,
						ExternalID: tryjob.MustBuildbucketID("host.com", 101),
						Result: &tryjob.Result{
							Status: tryjob.Result_FAILED_TRANSIENTLY,
						},
						Status: tryjob.Status_ENDED,
					},
				}
				updateAttempts(execState, failedTryjobs)
				retries := computeRetries(ctx, execState, failedTryjobs)
				So(retries, ShouldHaveLength, 0)
				So(execState.Status, ShouldEqual, tryjob.ExecutionState_FAILED)
				So(execState.FailureReason, ShouldContainSubstring, "Failed Tryjobs:")
				So(execState.FailureReason, ShouldContainSubstring, failedTryjobs[101].ExternalID.MustURL())
			})
		})
		Convey("with retry config", func() {
			execState.Requirement.RetryConfig = &cfgpb.Verifiers_Tryjob_RetryConfig{
				SingleQuota:            2,
				GlobalQuota:            3,
				FailureWeight:          3,
				TransientFailureWeight: 1,
				TimeoutWeight:          2,
			}
			Convey("quota", func() {
				Convey("for single execution exceeded", func() {
					failedTryjobs := makeFailedTryjobs(101)
					updateAttempts(execState, failedTryjobs)
					retries := computeRetries(ctx, execState, failedTryjobs)
					So(execState.FailureReason, ShouldEqual, "")
					So(retries, ShouldHaveLength, 1)

					addAttempt(execState.Executions[0])

					failedTryjobs = makeFailedTryjobs(102)
					updateAttempts(execState, failedTryjobs)
					retries = computeRetries(ctx, execState, failedTryjobs)
					So(execState.FailureReason, ShouldEqual, "")
					So(retries, ShouldHaveLength, 1)

					addAttempt(execState.Executions[0])

					failedTryjobs = makeFailedTryjobs(103)
					updateAttempts(execState, failedTryjobs)
					retries = computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 0)
					So(execState.FailureReason, ShouldContainSubstring, "Failed Tryjobs:")
					So(execState.FailureReason, ShouldContainSubstring, failedTryjobs[103].ExternalID.MustURL())
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_FAILED)
				})
				Convey("for whole run exceeded", func() {
					failedTryjobs := makeFailedTryjobs(101)
					updateAttempts(execState, failedTryjobs)
					retries := computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 1)

					addAttempt(execState.Executions[0])

					failedTryjobs = makeFailedTryjobs(201)
					updateAttempts(execState, failedTryjobs)
					retries = computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 1)

					addAttempt(execState.Executions[1])

					failedTryjobs = makeFailedTryjobs(202)
					updateAttempts(execState, failedTryjobs)
					retries = computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 1)

					addAttempt(execState.Executions[1])

					failedTryjobs = makeFailedTryjobs(102)
					updateAttempts(execState, failedTryjobs)
					retries = computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 0)
					So(execState.FailureReason, ShouldContainSubstring, "Failed Tryjobs:")
					So(execState.FailureReason, ShouldContainSubstring, failedTryjobs[102].ExternalID.MustURL())
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_FAILED)
				})
				Convey("reused run does not cause exceeding", func() {
					// Same as above, but the first attempt of the second
					// execution is a reused tryjob
					execState.Executions[1].Attempts[0].Reused = true

					failedTryjobs := makeFailedTryjobs(101)
					updateAttempts(execState, failedTryjobs)
					retries := computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 1)

					addAttempt(execState.Executions[0])

					failedTryjobs = makeFailedTryjobs(201)
					updateAttempts(execState, failedTryjobs)
					retries = computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 1)

					addAttempt(execState.Executions[1])

					failedTryjobs = makeFailedTryjobs(202)
					updateAttempts(execState, failedTryjobs)
					retries = computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 1)

					addAttempt(execState.Executions[1])

					failedTryjobs = makeFailedTryjobs(102)
					updateAttempts(execState, failedTryjobs)
					retries = computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 1)
					So(execState.Status, ShouldNotEqual, tryjob.ExecutionState_FAILED)
				})
			})
			Convey("with retry-denied property output", func() {
				Convey("on critical", func() {
					failedTryjobs := makeFailedTryjobs(101)
					failedTryjobs[101].Result.Output = &recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED}
					updateAttempts(execState, failedTryjobs)
					retries := computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 0)
					So(execState.FailureReason, ShouldContainSubstring, "Failed Tryjobs:")
					So(execState.FailureReason, ShouldContainSubstring, failedTryjobs[101].ExternalID.MustURL())
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_FAILED)
				})
				Convey("on experimental", func() {
					failedTryjobs := makeFailedTryjobs(301)
					failedTryjobs[301].Result.Output = &recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED}
					updateAttempts(execState, failedTryjobs)
					retries := computeRetries(ctx, execState, failedTryjobs)
					So(retries, ShouldHaveLength, 0)
					So(execState.FailureReason, ShouldEqual, "")
					So(execState.Status, ShouldNotEqual, tryjob.ExecutionState_FAILED)
				})
			})
			Convey("partial update only", func() {
				failedTryjobs := makeFailedTryjobs(101, 404, 505, 506)
				updateAttempts(execState, failedTryjobs)
				retries := computeRetries(ctx, execState, failedTryjobs)
				So(execState.Status, ShouldNotEqual, tryjob.ExecutionState_FAILED)
				So(execState.FailureReason, ShouldEqual, "")
				So(retries, ShouldHaveLength, 1)
				retry := retries[0]
				So(retry.defintion, ShouldResembleProto, makeDefinition("builder-one", true))
				So(retry.execution, ShouldResembleProto, &tryjob.ExecutionState_Execution{
					Attempts: []*tryjob.ExecutionState_Execution_Attempt{
						{
							TryjobId: 101,
							Status:   tryjob.Status_ENDED,
							Result: &tryjob.Result{
								Status: tryjob.Result_FAILED_TRANSIENTLY,
							},
						},
					},
					UsedQuota: 1,
				})
			})
			Convey("retries contains expected info", func() {
				failedTryjobs := makeFailedTryjobs(101)
				updateAttempts(execState, failedTryjobs)
				retries := computeRetries(ctx, execState, failedTryjobs)
				So(retries, ShouldHaveLength, 1)
				retry := retries[0]
				So(retry.defintion, ShouldResembleProto, makeDefinition("builder-one", true))
				So(retry.execution, ShouldResembleProto, &tryjob.ExecutionState_Execution{
					Attempts: []*tryjob.ExecutionState_Execution_Attempt{
						{
							TryjobId: 101,
							Status:   tryjob.Status_ENDED,
							Result: &tryjob.Result{
								Status: tryjob.Result_FAILED_TRANSIENTLY,
							},
						},
					},
					UsedQuota: 1,
				})
			})
		})
	})
}

func addAttempt(exec *tryjob.ExecutionState_Execution) {
	var prevTryjobID int64
	if len(exec.Attempts) > 0 {
		prevTryjobID = exec.Attempts[len(exec.Attempts)-1].TryjobId
	}
	exec.Attempts = append(exec.Attempts, &tryjob.ExecutionState_Execution_Attempt{
		TryjobId: prevTryjobID + 1,
		Status:   tryjob.Status_PENDING,
	})
}

func makeFailedTryjobs(ids ...int64) map[common.TryjobID]*tryjob.Tryjob {
	ret := make(map[common.TryjobID]*tryjob.Tryjob, len(ids))
	for _, id := range ids {
		ret[common.TryjobID(id)] = &tryjob.Tryjob{
			ID:         common.TryjobID(id),
			ExternalID: tryjob.MustBuildbucketID("host.com", id),
			Result: &tryjob.Result{
				Status: tryjob.Result_FAILED_TRANSIENTLY,
			},
			Status: tryjob.Status_ENDED,
		}
	}
	return ret
}
