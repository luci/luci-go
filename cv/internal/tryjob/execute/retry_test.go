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
	"math"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestCanRetryAll(t *testing.T) {
	ftt.Run("CanRetryAll", t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
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
		executor := &Executor{}

		t.Run("With no retry config", func(t *ftt.Test) {
			check := func(t testing.TB) {
				t.Helper()
				ok := executor.canRetryAll(ctx, execState, []int{0})
				assert.Loosely(t, ok, should.BeFalse)
				assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
					{
						Time: timestamppb.New(clock.Now(ctx).UTC()),
						Kind: &tryjob.ExecutionLogEntry_RetryDenied_{
							RetryDenied: &tryjob.ExecutionLogEntry_RetryDenied{
								Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
									{
										Definition: makeDefinition(builderZero, true),
										Id:         1,
										ExternalId: string(tryjob.MustBuildbucketID("buildbucket.example.com", math.MaxInt64-1)),
										Status:     tryjob.Status_ENDED,
										Result: &tryjob.Result{
											Status: tryjob.Result_FAILED_TRANSIENTLY,
										},
									},
								},
								Reason: "retry is not enabled in the config",
							},
						},
					},
				}))

			}

			t.Run("Nil config", func(t *ftt.Test) {
				execState = newExecStateBuilder(execState).
					withRetryConfig(nil).
					appendAttempt(builderZero, makeAttempt(1, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)).
					build()
				check(t)
			})
			t.Run("Empty config", func(t *ftt.Test) {
				execState = newExecStateBuilder(execState).
					withRetryConfig(&cfgpb.Verifiers_Tryjob_RetryConfig{}).
					appendAttempt(builderZero, makeAttempt(1, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)).
					build()
				check(t)
			})
		})
		t.Run("With retry config", func(t *ftt.Test) {
			t.Run("quota", func(t *ftt.Test) {
				t.Run("allows retry", func(t *ftt.Test) {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)).
						build()
					ok := executor.canRetryAll(ctx, execState, []int{0})
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, executor.logEntries, should.BeEmpty)
				})
				t.Run("for single execution exceeded", func(t *ftt.Test) {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)).
						build()
					ok := executor.canRetryAll(ctx, execState, []int{0})
					assert.Loosely(t, ok, should.BeFalse)
					assert.Loosely(t, execState.GetExecutions()[0].GetUsedQuota(), should.Equal(3))
					assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
						{
							Time: timestamppb.New(clock.Now(ctx).UTC()),
							Kind: &tryjob.ExecutionLogEntry_RetryDenied_{
								RetryDenied: &tryjob.ExecutionLogEntry_RetryDenied{
									Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
										{
											Definition: makeDefinition(builderZero, true),
											Id:         345,
											ExternalId: string(tryjob.MustBuildbucketID("buildbucket.example.com", math.MaxInt64-345)),
											Status:     tryjob.Status_ENDED,
											Result: &tryjob.Result{
												Status: tryjob.Result_FAILED_PERMANENTLY,
											},
										},
									},
									Reason: "insufficient quota",
								},
							},
						},
					}))
				})
				t.Run("for whole run exceeded", func(t *ftt.Test) {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_TIMEOUT)).
						appendAttempt(builderOne, makeAttempt(567, tryjob.Status_ENDED, tryjob.Result_TIMEOUT)).
						build()
					ok := executor.canRetryAll(ctx, execState, []int{0, 1})
					assert.Loosely(t, ok, should.BeFalse)
					assert.Loosely(t, execState.GetExecutions()[0].GetUsedQuota(), should.Equal(2))
					assert.Loosely(t, execState.GetExecutions()[1].GetUsedQuota(), should.Equal(2))
					assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
						{
							Time: timestamppb.New(clock.Now(ctx).UTC()),
							Kind: &tryjob.ExecutionLogEntry_RetryDenied_{
								RetryDenied: &tryjob.ExecutionLogEntry_RetryDenied{
									Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
										{
											Definition: makeDefinition(builderZero, true),
											Id:         345,
											ExternalId: string(tryjob.MustBuildbucketID("buildbucket.example.com", math.MaxInt64-345)),
											Status:     tryjob.Status_ENDED,
											Result: &tryjob.Result{
												Status: tryjob.Result_TIMEOUT,
											},
										},
										{
											Definition: makeDefinition(builderOne, true),
											Id:         567,
											ExternalId: string(tryjob.MustBuildbucketID("buildbucket.example.com", math.MaxInt64-567)),
											Status:     tryjob.Status_ENDED,
											Result: &tryjob.Result{
												Status: tryjob.Result_TIMEOUT,
											},
										},
									},
									Reason: "insufficient global quota",
								},
							},
						},
					}))
				})
				t.Run("reused run does not cause exceeding", func(t *ftt.Test) {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)).
						build()
					execState.GetExecutions()[0].Attempts[0].Reused = true
					ok := executor.canRetryAll(ctx, execState, []int{0})
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, execState.GetExecutions()[0].GetUsedQuota(), should.BeZero)
					assert.Loosely(t, executor.logEntries, should.BeEmpty)
				})
				t.Run("with retry-denied property output", func(t *ftt.Test) {
					execState = newExecStateBuilder(execState).
						appendAttempt(builderZero, makeAttempt(345, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)).
						build()
					execState.GetExecutions()[0].Attempts[0].Result.Output = &recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED}
					ok := executor.canRetryAll(ctx, execState, []int{0})
					assert.Loosely(t, ok, should.BeFalse)
					assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
						{
							Time: timestamppb.New(clock.Now(ctx).UTC()),
							Kind: &tryjob.ExecutionLogEntry_RetryDenied_{
								RetryDenied: &tryjob.ExecutionLogEntry_RetryDenied{
									Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
										{
											Definition: makeDefinition(builderZero, true),
											Id:         345,
											ExternalId: string(tryjob.MustBuildbucketID("buildbucket.example.com", math.MaxInt64-345)),
											Status:     tryjob.Status_ENDED,
											Result: &tryjob.Result{
												Status: tryjob.Result_FAILED_TRANSIENTLY,
												Output: &recipe.Output{
													Retry: recipe.Output_OUTPUT_RETRY_DENIED,
												},
											},
										},
									},
									Reason: "tryjob explicitly denies retry in its output",
								},
							},
						},
					}))
				})
			})
		})
	})
}
