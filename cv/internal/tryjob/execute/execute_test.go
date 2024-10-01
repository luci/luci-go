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
	"fmt"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/api/recipe/v1"
	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestPrepExecutionPlan(t *testing.T) {
	t.Parallel()
	ftt.Run("PrepExecutionPlan", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		executor := &Executor{
			Env: ct.Env,
		}

		t.Run("Tryjobs Updated", func(t *ftt.Test) {
			const builderFoo = "foo"
			r := &run.Run{
				Mode: run.FullRun,
			}
			prepPlan := func(execState *tryjob.ExecutionState, updatedTryjobs ...int64) *plan {
				_, p, err := executor.prepExecutionPlan(ctx, execState, r, updatedTryjobs, false)
				assert.Loosely(t, err, should.BeNil)
				return p
			}
			t.Run("Updates", func(t *ftt.Test) {
				const prevTryjobID = 101
				const curTryjobID = 102
				builderFooDef := makeDefinition(builderFoo, true)
				execState := newExecStateBuilder().
					appendDefinition(builderFooDef).
					appendAttempt(builderFoo, makeAttempt(prevTryjobID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)).
					appendAttempt(builderFoo, makeAttempt(curTryjobID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)).
					build()
				t.Run("Updates previous attempts", func(t *ftt.Test) {
					ensureTryjob(t, ctx, prevTryjobID, builderFooDef, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					plan := prepPlan(execState, prevTryjobID)
					assert.Loosely(t, plan.isEmpty(), should.BeTrue)
					assert.Loosely(t, execState, should.Resemble(newExecStateBuilder().
						appendDefinition(builderFooDef).
						appendAttempt(builderFoo, makeAttempt(prevTryjobID, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)).
						appendAttempt(builderFoo, makeAttempt(curTryjobID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)).
						build()))
				})
				t.Run("Ignore For not relevant Tryjob", func(t *ftt.Test) {
					original := proto.Clone(execState).(*tryjob.ExecutionState)
					const randomTryjob int64 = 567
					ensureTryjob(t, ctx, randomTryjob, builderFooDef, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					plan := prepPlan(execState, randomTryjob)
					assert.Loosely(t, plan.isEmpty(), should.BeTrue)
					assert.Loosely(t, execState, should.Resemble(original))
				})
				assert.Loosely(t, executor.logEntries, should.BeEmpty)
			})

			t.Run("Single Tryjob", func(t *ftt.Test) {
				const tjID = 101
				def := makeDefinition(builderFoo, true)
				execState := newExecStateBuilder().
					appendDefinition(def).
					appendAttempt(builderFoo, makeAttempt(tjID, tryjob.Status_PENDING, tryjob.Result_UNKNOWN)).
					withRetryConfig(&cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota:            2,
						GlobalQuota:            10,
						FailureWeight:          5,
						TransientFailureWeight: 1,
					}).
					build()

				t.Run("Succeeded", func(t *ftt.Test) {
					tj := ensureTryjob(t, ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					plan := prepPlan(execState, tjID)
					assert.Loosely(t, plan.isEmpty(), should.BeTrue)
					assert.Loosely(t, execState.Executions[0].Attempts, should.Resemble([]*tryjob.ExecutionState_Execution_Attempt{
						makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED),
					}))
					assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_SUCCEEDED))
					assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
						{
							Time: timestamppb.New(ct.Clock.Now().UTC()),
							Kind: &tryjob.ExecutionLogEntry_TryjobsEnded_{
								TryjobsEnded: &tryjob.ExecutionLogEntry_TryjobsEnded{
									Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
										makeLogTryjobSnapshot(def, tj, false),
									},
								},
							},
						},
					}))
				})

				t.Run("Succeeded but not reusable", func(t *ftt.Test) {
					ro := &recipe.Output{
						Reusability: &recipe.Output_Reusability{
							ModeAllowlist: []string{string(run.DryRun)},
						},
					}
					assert.Loosely(t, isModeAllowed(r.Mode, ro.GetReusability().GetModeAllowlist()), should.BeFalse)
					tj := ensureTryjob(t, ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					tj.Result.Output = ro
					assert.Loosely(t, datastore.Put(ctx, tj), should.BeNil)
					tryjob.LatestAttempt(execState.GetExecutions()[0]).Reused = true
					plan := prepPlan(execState, tjID)
					assert.Loosely(t, plan.isEmpty(), should.BeFalse)
					assert.Loosely(t, plan.triggerNewAttempt, should.HaveLength(1))
					assert.Loosely(t, plan.triggerNewAttempt[0].definition, should.Resemble(def))
					attempt := makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					attempt.Reused = true
					attempt.Result.Output = ro
					assert.Loosely(t, execState.Executions[0].Attempts, should.Resemble([]*tryjob.ExecutionState_Execution_Attempt{attempt}))
					assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_RUNNING))
					assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
						{
							Time: timestamppb.New(ct.Clock.Now().UTC()),
							Kind: &tryjob.ExecutionLogEntry_TryjobsEnded_{
								TryjobsEnded: &tryjob.ExecutionLogEntry_TryjobsEnded{
									Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
										makeLogTryjobSnapshot(def, tj, true),
									},
								},
							},
						},
					}))
				})

				t.Run("Still Running", func(t *ftt.Test) {
					tj := ensureTryjob(t, ctx, tjID, def, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
					t.Run("Tryjob is critical", func(t *ftt.Test) {
						plan := prepPlan(execState, tjID)
						assert.Loosely(t, plan.isEmpty(), should.BeTrue)
						assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_RUNNING))
						assert.Loosely(t, execState.Executions[0].Attempts, should.Resemble([]*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN),
						}))
						t.Run("but no longer reusable", func(t *ftt.Test) {
							ro := &recipe.Output{
								Reusability: &recipe.Output_Reusability{
									ModeAllowlist: []string{string(run.DryRun)},
								},
							}
							assert.Loosely(t, isModeAllowed(r.Mode, ro.GetReusability().GetModeAllowlist()), should.BeFalse)
							tj.Result.Output = ro
							assert.Loosely(t, datastore.Put(ctx, tj), should.BeNil)
							tryjob.LatestAttempt(execState.GetExecutions()[0]).Reused = true
							plan := prepPlan(execState, tjID)
							assert.Loosely(t, plan.isEmpty(), should.BeFalse)
							assert.Loosely(t, plan.triggerNewAttempt, should.HaveLength(1))
							assert.Loosely(t, plan.triggerNewAttempt[0].definition, should.Resemble(def))
							attempt := makeAttempt(tjID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
							attempt.Reused = true
							attempt.Result.Output = ro
							assert.Loosely(t, execState.Executions[0].Attempts, should.Resemble([]*tryjob.ExecutionState_Execution_Attempt{attempt}))
							assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_RUNNING))
						})
					})
					t.Run("Tryjob is not critical", func(t *ftt.Test) {
						execState.GetRequirement().GetDefinitions()[0].Critical = false
						plan := prepPlan(execState, tjID)
						assert.Loosely(t, plan.isEmpty(), should.BeTrue)
						assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_SUCCEEDED))
						assert.Loosely(t, execState.Executions[0].Attempts, should.Resemble([]*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN),
						}))
					})
					assert.Loosely(t, executor.logEntries, should.BeEmpty)
				})

				t.Run("Failed", func(t *ftt.Test) {
					var tj *tryjob.Tryjob
					t.Run("Critical and can retry", func(t *ftt.Test) {
						// Quota allows retrying transient failure.
						tj = ensureTryjob(t, ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
						plan := prepPlan(execState, tjID)
						assert.Loosely(t, plan.isEmpty(), should.BeFalse)
						assert.Loosely(t, plan.triggerNewAttempt, should.HaveLength(1))
						assert.Loosely(t, plan.triggerNewAttempt[0].definition, should.Resemble(execState.GetRequirement().GetDefinitions()[0]))
						assert.Loosely(t, execState.Executions[0].Attempts, should.Resemble([]*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY),
						}))
						assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_RUNNING))
						assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
							{
								Time: timestamppb.New(ct.Clock.Now().UTC()),
								Kind: &tryjob.ExecutionLogEntry_TryjobsEnded_{
									TryjobsEnded: &tryjob.ExecutionLogEntry_TryjobsEnded{
										Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
											makeLogTryjobSnapshot(def, tj, false),
										},
									},
								},
							},
						}))
					})
					t.Run("Critical and can NOT retry", func(t *ftt.Test) {
						// Quota doesn't allow retrying permanent failure.
						tj = ensureTryjob(t, ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)
						plan := prepPlan(execState, tjID)
						assert.Loosely(t, plan.isEmpty(), should.BeTrue)
						assert.Loosely(t, execState.Executions[0].Attempts, should.Resemble([]*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY),
						}))
						assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_FAILED))
						assert.Loosely(t, execState.Failures, should.Resemble(&tryjob.ExecutionState_Failures{
							UnsuccessfulResults: []*tryjob.ExecutionState_Failures_UnsuccessfulResult{
								{TryjobId: tjID},
							},
						}))
						assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
							{
								Time: timestamppb.New(ct.Clock.Now().UTC()),
								Kind: &tryjob.ExecutionLogEntry_TryjobsEnded_{
									TryjobsEnded: &tryjob.ExecutionLogEntry_TryjobsEnded{
										Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
											makeLogTryjobSnapshot(def, tj, false),
										},
									},
								},
							},
							{
								Time: timestamppb.New(ct.Clock.Now().UTC()),
								Kind: &tryjob.ExecutionLogEntry_RetryDenied_{
									RetryDenied: &tryjob.ExecutionLogEntry_RetryDenied{
										Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
											makeLogTryjobSnapshot(def, tj, false),
										},
										Reason: "insufficient quota",
									},
								},
							},
						}))
					})
					t.Run("Tryjob is not critical", func(t *ftt.Test) {
						tj = ensureTryjob(t, ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)
						execState.GetRequirement().GetDefinitions()[0].Critical = false
						plan := prepPlan(execState, tjID)
						assert.Loosely(t, plan.isEmpty(), should.BeTrue)
						assert.Loosely(t, execState.Executions[0].Attempts, should.Resemble([]*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY),
						}))
						// Still consider execution as succeeded even though non-critical
						// tryjob has failed.
						assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_SUCCEEDED))
						assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
							{
								Time: timestamppb.New(ct.Clock.Now().UTC()),
								Kind: &tryjob.ExecutionLogEntry_TryjobsEnded_{
									TryjobsEnded: &tryjob.ExecutionLogEntry_TryjobsEnded{
										Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
											makeLogTryjobSnapshot(def, tj, false),
										},
									},
								},
							},
						}))
					})
				})

				t.Run("Untriggered", func(t *ftt.Test) {
					ensureTryjob(t, ctx, tjID, def, tryjob.Status_UNTRIGGERED, tryjob.Result_UNKNOWN)
					t.Run("Reused", func(t *ftt.Test) {
						tryjob.LatestAttempt(execState.GetExecutions()[0]).Reused = true
						plan := prepPlan(execState, tjID)
						assert.Loosely(t, plan.isEmpty(), should.BeFalse)
						assert.Loosely(t, plan.triggerNewAttempt, should.HaveLength(1))
						assert.Loosely(t, plan.triggerNewAttempt[0].definition, should.Resemble(execState.GetRequirement().GetDefinitions()[0]))
						attempt := makeAttempt(tjID, tryjob.Status_UNTRIGGERED, tryjob.Result_UNKNOWN)
						attempt.Reused = true
						assert.Loosely(t, execState.Executions[0].Attempts, should.Resemble([]*tryjob.ExecutionState_Execution_Attempt{attempt}))
						assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_RUNNING))
					})
					t.Run("Not critical", func(t *ftt.Test) {
						execState.GetRequirement().GetDefinitions()[0].Critical = false
						plan := prepPlan(execState, tjID)
						assert.Loosely(t, plan.isEmpty(), should.BeTrue)
					})
					assert.Loosely(t, executor.logEntries, should.BeEmpty)
				})
			})

			t.Run("Multiple Tryjobs", func(t *ftt.Test) {
				builder1Def := makeDefinition("builder1", true)
				builder2Def := makeDefinition("builder2", false)
				builder3Def := makeDefinition("builder3", true)
				execState := newExecStateBuilder().
					appendDefinition(builder1Def).
					appendAttempt("builder1", makeAttempt(101, tryjob.Status_PENDING, tryjob.Result_UNKNOWN)).
					appendDefinition(builder2Def).
					appendAttempt("builder2", makeAttempt(201, tryjob.Status_PENDING, tryjob.Result_UNKNOWN)).
					appendDefinition(builder3Def).
					appendAttempt("builder3", makeAttempt(301, tryjob.Status_PENDING, tryjob.Result_UNKNOWN)).
					withRetryConfig(&cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota:            2,
						GlobalQuota:            10,
						FailureWeight:          5,
						TransientFailureWeight: 1,
					}).
					build()

				t.Run("Has non-ended critical Tryjobs", func(t *ftt.Test) {
					ensureTryjob(t, ctx, 101, builder1Def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					ensureTryjob(t, ctx, 201, builder2Def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					// 301 has not completed.
					plan := prepPlan(execState, 101, 201)
					assert.Loosely(t, plan.isEmpty(), should.BeTrue)
					assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_RUNNING))
				})
				t.Run("Execution ends when all critical tryjob ended", func(t *ftt.Test) {
					ensureTryjob(t, ctx, 101, builder1Def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					// 201 (non-critical) has not completed.
					ensureTryjob(t, ctx, 301, builder3Def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					plan := prepPlan(execState, 101, 301)
					assert.Loosely(t, plan.isEmpty(), should.BeTrue)
					assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_SUCCEEDED))
				})
				t.Run("Failed critical Tryjob fails the execution", func(t *ftt.Test) {
					ensureTryjob(t, ctx, 101, builder1Def, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)
					// One critical tryjob is still running.
					ensureTryjob(t, ctx, 301, builder3Def, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
					plan := prepPlan(execState, 101, 301)
					assert.Loosely(t, plan.isEmpty(), should.BeTrue)
					assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_FAILED))
					assert.Loosely(t, execState.Failures, should.Resemble(&tryjob.ExecutionState_Failures{
						UnsuccessfulResults: []*tryjob.ExecutionState_Failures_UnsuccessfulResult{
							{TryjobId: 101},
						},
					}))
				})
				t.Run("Can retry multiple", func(t *ftt.Test) {
					ensureTryjob(t, ctx, 101, builder1Def, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					// 201 has not completed.
					ensureTryjob(t, ctx, 301, builder3Def, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					plan := prepPlan(execState, 101, 301)
					assert.Loosely(t, plan.isEmpty(), should.BeFalse)
					assert.Loosely(t, plan.triggerNewAttempt, should.HaveLength(2))
					assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_RUNNING))
				})
			})
		})

		t.Run("Requirement Changed", func(t *ftt.Test) {
			var builderFooDef = makeDefinition("builderFoo", true)
			latestReqmt := &tryjob.Requirement{
				Definitions: []*tryjob.Definition{
					builderFooDef,
				},
			}
			r := &run.Run{
				Tryjobs: &run.Tryjobs{
					Requirement:           latestReqmt,
					RequirementVersion:    2,
					RequirementComputedAt: timestamppb.New(ct.Clock.Now()),
				},
			}

			t.Run("No-op if current version newer than target version", func(t *ftt.Test) {
				execState := newExecStateBuilder().
					withRequirementVersion(int(r.Tryjobs.GetRequirementVersion()) + 1).
					build()
				_, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, plan.isEmpty(), should.BeTrue)
				assert.Loosely(t, executor.logEntries, should.BeEmpty)
			})

			t.Run("New Requirement", func(t *ftt.Test) {
				execState := newExecStateBuilder().build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, execState.Requirement, should.Resemble(latestReqmt))
				assert.Loosely(t, plan.triggerNewAttempt, should.HaveLength(1))
				assert.Loosely(t, plan.triggerNewAttempt[0].definition, should.Resemble(builderFooDef))
				assert.Loosely(t, executor.logEntries, should.BeEmpty)
			})

			t.Run("Removed Tryjob", func(t *ftt.Test) {
				removedDef := makeDefinition("removed", true)
				execState := newExecStateBuilder().
					appendDefinition(removedDef).
					appendDefinition(builderFooDef).
					build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, execState.Requirement, should.Resemble(latestReqmt))
				assert.Loosely(t, plan.triggerNewAttempt, should.BeEmpty)
				assert.Loosely(t, plan.discard, should.HaveLength(1))
				assert.Loosely(t, plan.discard[0].definition, should.Resemble(removedDef))
				assert.Loosely(t, execState.Executions, should.HaveLength(1))
				assert.Loosely(t, plan.discard[0].discardReason, should.Equal(noLongerRequiredInConfig))
				assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
					{
						Time: timestamppb.New(ct.Clock.Now().UTC()),
						Kind: &tryjob.ExecutionLogEntry_RequirementChanged_{
							RequirementChanged: &tryjob.ExecutionLogEntry_RequirementChanged{},
						},
					},
				}))
			})

			t.Run("Changed Tryjob", func(t *ftt.Test) {
				builderFooOriginal := proto.Clone(builderFooDef).(*tryjob.Definition)
				builderFooOriginal.DisableReuse = !builderFooDef.DisableReuse
				execState := newExecStateBuilder().
					appendDefinition(builderFooOriginal).
					build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, execState.Requirement, should.Resemble(latestReqmt))
				assert.Loosely(t, plan.isEmpty(), should.BeTrue)
				assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
					{
						Time: timestamppb.New(ct.Clock.Now().UTC()),
						Kind: &tryjob.ExecutionLogEntry_RequirementChanged_{
							RequirementChanged: &tryjob.ExecutionLogEntry_RequirementChanged{},
						},
					},
				}))
			})

			t.Run("Change in retry config", func(t *ftt.Test) {
				execState := newExecStateBuilder().
					appendDefinition(builderFooDef).
					withRetryConfig(&cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 10,
					}).
					build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, execState.Requirement, should.Resemble(latestReqmt))
				assert.Loosely(t, plan.isEmpty(), should.BeTrue)
				assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
					{
						Time: timestamppb.New(ct.Clock.Now().UTC()),
						Kind: &tryjob.ExecutionLogEntry_RequirementChanged_{
							RequirementChanged: &tryjob.ExecutionLogEntry_RequirementChanged{},
						},
					},
				}))
			})

			t.Run("Empty definitions", func(t *ftt.Test) {
				latestReqmt.Definitions = nil
				execState := newExecStateBuilder().
					appendDefinition(builderFooDef).
					build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, execState.Requirement, should.Resemble(latestReqmt))
				assert.Loosely(t, plan.discard, should.HaveLength(1))
				assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_SUCCEEDED))
			})
		})
	})
}

func TestExecutePlan(t *testing.T) {
	t.Parallel()
	ftt.Run("ExecutePlan", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		t.Run("Discard tryjobs", func(t *ftt.Test) {
			executor := &Executor{
				Env: ct.Env,
			}
			const bbHost = "buildbucket.example.com"
			def := &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: bbHost,
						Builder: &bbpb.BuilderID{
							Project: "some_proj",
							Bucket:  "some_bucket",
							Builder: "some_builder",
						},
					},
				},
			}
			_, err := executor.executePlan(ctx, &plan{
				discard: []planItem{
					{
						definition: def,
						execution: &tryjob.ExecutionState_Execution{
							Attempts: []*tryjob.ExecutionState_Execution_Attempt{
								{
									TryjobId:   67,
									ExternalId: string(tryjob.MustBuildbucketID(bbHost, 6767)),
								},
								{
									TryjobId:   58,
									ExternalId: string(tryjob.MustBuildbucketID(bbHost, 5858)),
								},
							},
						},
						discardReason: "no longer needed",
					},
				},
			}, nil, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, executor.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
				{
					Time: timestamppb.New(ct.Clock.Now().UTC()),
					Kind: &tryjob.ExecutionLogEntry_TryjobDiscarded_{
						TryjobDiscarded: &tryjob.ExecutionLogEntry_TryjobDiscarded{
							Snapshot: &tryjob.ExecutionLogEntry_TryjobSnapshot{
								Definition: def,
								Id:         58,
								ExternalId: string(tryjob.MustBuildbucketID(bbHost, 5858)),
							},
							Reason: "no longer needed",
						},
					},
				},
			}))
		})

		t.Run("Trigger new attempt", func(t *ftt.Test) {
			const (
				lProject        = "test_proj"
				configGroupName = "test_config_group"
				bbHost          = "buildbucket.example.com"
				buildID         = 9524107902457
				clid            = 34586452134
				gHost           = "example-review.com"
				gRepo           = "repo/a"
				gChange         = 123
				gPatchset       = 5
				gMinPatchset    = 4
			)
			now := ct.Clock.Now().UTC()
			var runID = common.MakeRunID(lProject, now.Add(-1*time.Hour), 1, []byte("abcd"))
			r := &run.Run{
				ID:            runID,
				ConfigGroupID: prjcfg.MakeConfigGroupID("deedbeef", configGroupName),
				Mode:          run.DryRun,
				CLs:           common.CLIDs{clid},
				Status:        run.Status_RUNNING,
				CreateTime:    now.Add(-1 * time.Hour),
				StartTime:     now.Add(-30 * time.Minute),
			}
			runCL := &run.RunCL{
				ID:  clid,
				Run: datastore.NewKey(ctx, common.RunKind, string(runID), 0, nil),
				Detail: &changelist.Snapshot{
					Patchset:              gPatchset,
					MinEquivalentPatchset: gMinPatchset,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: &gerritpb.ChangeInfo{
								Project: gRepo,
								Number:  gChange,
								Owner: &gerritpb.AccountInfo{
									Email: "owner@example.com",
								},
							},
						},
					},
				},
				Trigger: &run.Trigger{
					Mode:  string(run.DryRun),
					Email: "triggerer@example.com",
				},
			}
			assert.Loosely(t, datastore.Put(ctx, runCL), should.BeNil)
			builder := &bbpb.BuilderID{
				Project: lProject,
				Bucket:  "some_bucket",
				Builder: "some_builder",
			}
			def := &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host:    bbHost,
						Builder: builder,
					},
				},
				Critical: true,
			}
			ct.BuildbucketFake.AddBuilder(bbHost, builder, nil)
			executor := &Executor{
				Env: ct.Env,
				Backend: &bbfacade.Facade{
					ClientFactory: ct.BuildbucketFake.NewClientFactory(),
				},
				RM: run.NewNotifier(ct.TQDispatcher),
			}
			execution := &tryjob.ExecutionState_Execution{}
			execState := &tryjob.ExecutionState{
				Executions: []*tryjob.ExecutionState_Execution{execution},
				Requirement: &tryjob.Requirement{
					Definitions: []*tryjob.Definition{def},
				},
				Status: tryjob.ExecutionState_RUNNING,
			}

			t.Run("New Tryjob is not launched when tryjob can be reused", func(t *ftt.Test) {
				result := &tryjob.Result{
					CreateTime: timestamppb.New(now.Add(-staleTryjobAge / 2)),
					Backend: &tryjob.Result_Buildbucket_{
						Buildbucket: &tryjob.Result_Buildbucket{
							Id:      buildID,
							Builder: builder,
						},
					},
					Status: tryjob.Result_SUCCEEDED,
				}
				reuseTryjob := tryjob.MustBuildbucketID(bbHost, buildID).MustCreateIfNotExists(ctx)
				reuseTryjob.EVersion = 1
				reuseTryjob.EntityCreateTime = now.Add(-staleTryjobAge / 2)
				reuseTryjob.EntityUpdateTime = now.Add(-1 * time.Minute)
				reuseTryjob.ReuseKey = computeReuseKey([]*run.RunCL{runCL})
				reuseTryjob.CLPatchsets = tryjob.CLPatchsets{tryjob.MakeCLPatchset(runCL.ID, gPatchset)}
				reuseTryjob.Definition = def
				reuseTryjob.Status = tryjob.Status_ENDED
				reuseTryjob.LaunchedBy = common.MakeRunID(lProject, now.Add(-2*time.Hour), 1, []byte("efgh"))
				reuseTryjob.Result = result

				assert.That(t, datastore.Put(ctx, reuseTryjob), should.ErrLike(nil))
				execState, err := executor.executePlan(ctx, &plan{
					triggerNewAttempt: []planItem{
						{
							definition: def,
							execution:  execution,
						},
					},
				}, r, execState)
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, execState.GetStatus(), should.Equal(tryjob.ExecutionState_RUNNING))
				assert.That(t, execState.GetExecutions()[0].GetAttempts(), should.Match(
					[]*tryjob.ExecutionState_Execution_Attempt{
						{
							TryjobId:   int64(reuseTryjob.ID),
							ExternalId: string(tryjob.MustBuildbucketID(bbHost, buildID)),
							Status:     tryjob.Status_ENDED,
							Result:     result,
							Reused:     true,
						},
					}))
				assert.Loosely(t, executor.stagedMetricReportFns, should.BeEmpty)
			})

			t.Run("When Tryjob can't be reused, thus launch a new Tryjob", func(t *ftt.Test) {
				execState, err := executor.executePlan(ctx, &plan{
					triggerNewAttempt: []planItem{
						{
							definition: def,
							execution:  execution,
						},
					},
				}, r, execState)
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, execState.GetStatus(), should.Equal(tryjob.ExecutionState_RUNNING))
				assert.Loosely(t, execState.GetExecutions()[0].GetAttempts(), should.HaveLength(1))
				attempt := execState.GetExecutions()[0].GetAttempts()[0]
				assert.That(t, attempt.GetTryjobId(), should.NotEqual(int64(0)))
				assert.Loosely(t, attempt.GetExternalId(), should.NotBeEmpty)
				assert.That(t, attempt.GetStatus(), should.Equal(tryjob.Status_TRIGGERED))
				assert.That(t, attempt.GetReused(), should.BeFalse)
				assert.Loosely(t, executor.stagedMetricReportFns, should.HaveLength(2))
				executor.stagedMetricReportFns[0](ctx)
				tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, def, func(ctx context.Context) {
					assert.That(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobLaunched, lProject, configGroupName, true, false).(int64), should.Equal(int64(1)))
				})
				executor.stagedMetricReportFns[1](ctx)
				assert.That(t, ct.TSMonSentDistr(ctx, metrics.Internal.CreateToFirstTryjobLatency, lProject, configGroupName, string(r.Mode)).Sum(), should.AlmostEqual(float64(ct.Clock.Now().Sub(r.CreateTime).Milliseconds())))
				assert.That(t, ct.TSMonSentDistr(ctx, metrics.Internal.StartToFirstTryjobLatency, lProject, configGroupName, string(r.Mode)).Sum(), should.AlmostEqual(float64(ct.Clock.Now().Sub(r.StartTime).Milliseconds())))
			})

			t.Run("Fail the execution if encounter launch failure", func(t *ftt.Test) {
				def.GetBuildbucket().GetBuilder().Project = "another_proj"
				execState, err := executor.executePlan(ctx, &plan{
					triggerNewAttempt: []planItem{
						{
							definition: def,
							execution:  execution,
						},
					},
				}, r, execState)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, execState.GetExecutions()[0].GetAttempts(), should.HaveLength(1))
				attempt := execState.GetExecutions()[0].GetAttempts()[0]
				assert.Loosely(t, attempt.GetTryjobId(), should.NotEqual(0))
				assert.Loosely(t, attempt.GetExternalId(), should.BeEmpty)
				assert.Loosely(t, attempt.GetStatus(), should.Equal(tryjob.Status_UNTRIGGERED))
				assert.Loosely(t, execState.Status, should.Equal(tryjob.ExecutionState_FAILED))
				assert.Loosely(t, execState.Failures, should.Resemble(&tryjob.ExecutionState_Failures{
					LaunchFailures: []*tryjob.ExecutionState_Failures_LaunchFailure{
						{
							Definition: def,
							Reason:     "received NotFound from buildbucket. message: builder another_proj/some_bucket/some_builder not found",
						},
					},
				}))
				assert.Loosely(t, executor.stagedMetricReportFns, should.BeEmpty)
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
			state: initExecutionState(),
		}
	case 1:
		return &execStateBuilder{
			state: original[0],
		}
	default:
		panic(fmt.Errorf("more than one original execState provided"))
	}
}

func (esb *execStateBuilder) withStatus(status tryjob.ExecutionState_Status) *execStateBuilder {
	esb.state.Status = status
	return esb
}

func (esb *execStateBuilder) appendDefinition(def *tryjob.Definition) *execStateBuilder {
	esb.ensureRequirement()
	esb.state.Requirement.Definitions = append(esb.state.Requirement.Definitions, def)
	esb.state.Executions = append(esb.state.Executions, &tryjob.ExecutionState_Execution{})
	return esb
}

func (esb *execStateBuilder) withRetryConfig(retryConfig *cfgpb.Verifiers_Tryjob_RetryConfig) *execStateBuilder {
	esb.ensureRequirement()
	esb.state.Requirement.RetryConfig = proto.Clone(retryConfig).(*cfgpb.Verifiers_Tryjob_RetryConfig)
	return esb
}

func (esb *execStateBuilder) withRequirementVersion(ver int) *execStateBuilder {
	esb.state.RequirementVersion = int32(ver)
	return esb
}

func (esb *execStateBuilder) appendAttempt(builderName string, attempt *tryjob.ExecutionState_Execution_Attempt) *execStateBuilder {
	esb.ensureRequirement()
	for i, def := range esb.state.Requirement.GetDefinitions() {
		if builderName == def.GetBuildbucket().GetBuilder().GetBuilder() {
			esb.state.Executions[i].Attempts = append(esb.state.Executions[i].Attempts, attempt)
			return esb
		}
	}
	panic(fmt.Errorf("can't find builder %s", builderName))
}

func (esb *execStateBuilder) ensureRequirement() {
	if esb.state.Requirement == nil {
		esb.state.Requirement = &tryjob.Requirement{}
	}
}

func (esb *execStateBuilder) build() *tryjob.ExecutionState {
	return esb.state
}

func makeDefinition(builder string, critical bool) *tryjob.Definition {
	return &tryjob.Definition{
		Backend: &tryjob.Definition_Buildbucket_{
			Buildbucket: &tryjob.Definition_Buildbucket{
				Host: "buildbucket.example.com",
				Builder: &bbpb.BuilderID{
					Project: "test",
					Bucket:  "bucket",
					Builder: builder,
				},
			},
		},
		Critical: critical,
	}
}

func makeAttempt(tjID int64, status tryjob.Status, resultStatus tryjob.Result_Status) *tryjob.ExecutionState_Execution_Attempt {
	return &tryjob.ExecutionState_Execution_Attempt{
		TryjobId:   int64(tjID),
		ExternalId: string(tryjob.MustBuildbucketID("buildbucket.example.com", math.MaxInt64-tjID)),
		Status:     status,
		Result: &tryjob.Result{
			Status: resultStatus,
		},
	}
}

func ensureTryjob(t testing.TB, ctx context.Context, id int64, def *tryjob.Definition, status tryjob.Status, resultStatus tryjob.Result_Status) *tryjob.Tryjob {
	t.Helper()
	now := datastore.RoundTime(clock.Now(ctx).UTC())
	tj := &tryjob.Tryjob{
		ID:               common.TryjobID(id),
		ExternalID:       tryjob.MustBuildbucketID("buildbucket.example.com", math.MaxInt64-id),
		Definition:       def,
		Status:           status,
		EntityCreateTime: now,
		EntityUpdateTime: now,
		Result: &tryjob.Result{
			Status: resultStatus,
		},
	}
	assert.Loosely(t, datastore.Put(ctx, tj), should.BeNil, truth.LineContext())
	return tj
}
