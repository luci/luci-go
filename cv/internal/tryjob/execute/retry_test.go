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
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCanRetryAll(t *testing.T) {
	Convey("CanRetryAll", t, func() {
		ctx := context.Background()
		const builderZero = "builder-zero"
		const builderOne = "builder-one"
		execState := newExecStateBuilder().
			appendDefinition(makeDefinition(builderZero, true)).
			appendDefinition(makeDefinition(builderOne, true)).
			withRetryConfig(&cfgpb.Verifiers_Tryjob_RetryConfig{
				SingleQuota:            2,
				GlobalQuota:            3,
				FailureWeight:          3,
				TransientFailureWeight: 1,
				TimeoutWeight:          2,
			}).
			build()

		Convey("With no retry config", func() {
			execState = newExecStateBuilder(execState).
				withRetryConfig(nil).
				appendAttempt(builderZero, makeAttempt(1, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)).
				build()
			ok, reason := canRetryAll(ctx, execState, []int{0})
			So(ok, ShouldBeFalse)
			So(reason, ShouldEqual, "retry is not enabled for this Config Group")
		})
		Convey("With retry config", func() {
			Convey("quota", func() {
				Convey("allows retry", func() {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)).
						build()
					ok, reason := canRetryAll(ctx, execState, []int{0})
					So(ok, ShouldBeTrue)
					So(reason, ShouldBeEmpty)
				})
				Convey("for single execution exceeded", func() {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)).
						build()
					ok, reason := canRetryAll(ctx, execState, []int{0})
					So(ok, ShouldBeFalse)
					So(reason, ShouldContainSubstring, "cannot be retried due to insufficient quota")
					So(execState.GetExecutions()[0].GetUsedQuota(), ShouldEqual, 3)
				})
				Convey("for whole run exceeded", func() {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_TIMEOUT)).
						appendAttempt(builderOne, makeAttempt(567, tryjob.Status_ENDED, tryjob.Result_TIMEOUT)).
						build()
					ok, reason := canRetryAll(ctx, execState, []int{0, 1})
					So(ok, ShouldBeFalse)
					So(reason, ShouldEqual, "tryjobs cannot be retried due to insufficient global quota")
					So(execState.GetExecutions()[0].GetUsedQuota(), ShouldEqual, 2)
					So(execState.GetExecutions()[1].GetUsedQuota(), ShouldEqual, 2)
				})
				Convey("reused run does not cause exceeding", func() {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)).
						build()
					execState.GetExecutions()[0].Attempts[0].Reused = true
					ok, reason := canRetryAll(ctx, execState, []int{0})
					So(ok, ShouldBeTrue)
					So(reason, ShouldBeEmpty)
					So(execState.GetExecutions()[0].GetUsedQuota(), ShouldEqual, 0)
				})
				Convey("with retry-denied property output", func() {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)).
						build()
					execState.GetExecutions()[0].Attempts[0].Result.Output = &recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED}
					ok, reason := canRetryAll(ctx, execState, []int{0})
					So(ok, ShouldBeFalse)
					So(reason, ShouldContainSubstring, "denies retry")
				})
			})
		})
	})
}
