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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPrepExecutionPlan(t *testing.T) {
	t.Parallel()
	Convey("PrepExecutionPlan", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		executor := &Executor{
			Env: ct.Env,
		}

		Convey("Tryjobs Updated", func() {
			const builderFoo = "foo"
			r := &run.Run{
				Mode: run.FullRun,
			}
			prepPlan := func(execState *tryjob.ExecutionState, updatedTryjobs ...int64) *plan {
				_, p, err := executor.prepExecutionPlan(ctx, execState, r, updatedTryjobs, false)
				So(err, ShouldBeNil)
				return p
			}
			Convey("Updates", func() {
				const prevTryjobID = 101
				const curTryjobID = 102
				builderFooDef := makeDefinition(builderFoo, true)
				execState := newExecStateBuilder().
					appendDefinition(builderFooDef).
					appendAttempt(builderFoo, makeAttempt(prevTryjobID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)).
					appendAttempt(builderFoo, makeAttempt(curTryjobID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)).
					build()
				Convey("Updates previous attempts", func() {
					ensureTryjob(ctx, prevTryjobID, builderFooDef, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					plan := prepPlan(execState, prevTryjobID)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState, ShouldResembleProto, newExecStateBuilder().
						appendDefinition(builderFooDef).
						appendAttempt(builderFoo, makeAttempt(prevTryjobID, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)).
						appendAttempt(builderFoo, makeAttempt(curTryjobID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)).
						build())
				})
				Convey("Ignore For not relevant Tryjob", func() {
					original := proto.Clone(execState).(*tryjob.ExecutionState)
					const randomTryjob int64 = 567
					ensureTryjob(ctx, randomTryjob, builderFooDef, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					plan := prepPlan(execState, randomTryjob)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState, ShouldResembleProto, original)
				})
				So(executor.logEntries, ShouldBeEmpty)
			})

			Convey("Single Tryjob", func() {
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

				Convey("Succeeded", func() {
					tj := ensureTryjob(ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					plan := prepPlan(execState, tjID)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
						makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED),
					})
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_SUCCEEDED)
					So(executor.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
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
					})
				})

				Convey("Succeeded but not reusable", func() {
					ro := &recipe.Output{
						Reusability: &recipe.Output_Reusability{
							ModeAllowlist: []string{string(run.DryRun)},
						},
					}
					So(isModeAllowed(r.Mode, ro.GetReusability().GetModeAllowlist()), ShouldBeFalse)
					tj := ensureTryjob(ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					tj.Result.Output = ro
					So(datastore.Put(ctx, tj), ShouldBeNil)
					tryjob.LatestAttempt(execState.GetExecutions()[0]).Reused = true
					plan := prepPlan(execState, tjID)
					So(plan.isEmpty(), ShouldBeFalse)
					So(plan.triggerNewAttempt, ShouldHaveLength, 1)
					So(plan.triggerNewAttempt[0].definition, ShouldResembleProto, def)
					attempt := makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					attempt.Reused = true
					attempt.Result.Output = ro
					So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{attempt})
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
					So(executor.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
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
					})
				})

				Convey("Still Running", func() {
					tj := ensureTryjob(ctx, tjID, def, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
					Convey("Tryjob is critical", func() {
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeTrue)
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
						So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN),
						})
						Convey("but no longer reusable", func() {
							ro := &recipe.Output{
								Reusability: &recipe.Output_Reusability{
									ModeAllowlist: []string{string(run.DryRun)},
								},
							}
							So(isModeAllowed(r.Mode, ro.GetReusability().GetModeAllowlist()), ShouldBeFalse)
							tj.Result.Output = ro
							So(datastore.Put(ctx, tj), ShouldBeNil)
							tryjob.LatestAttempt(execState.GetExecutions()[0]).Reused = true
							plan := prepPlan(execState, tjID)
							So(plan.isEmpty(), ShouldBeFalse)
							So(plan.triggerNewAttempt, ShouldHaveLength, 1)
							So(plan.triggerNewAttempt[0].definition, ShouldResembleProto, def)
							attempt := makeAttempt(tjID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
							attempt.Reused = true
							attempt.Result.Output = ro
							So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{attempt})
							So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
						})
					})
					Convey("Tryjob is not critical", func() {
						execState.GetRequirement().GetDefinitions()[0].Critical = false
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeTrue)
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_SUCCEEDED)
						So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN),
						})
					})
					So(executor.logEntries, ShouldBeEmpty)
				})

				Convey("Failed", func() {
					var tj *tryjob.Tryjob
					Convey("Critical and can retry", func() {
						// Quota allows retrying transient failure.
						tj = ensureTryjob(ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeFalse)
						So(plan.triggerNewAttempt, ShouldHaveLength, 1)
						So(plan.triggerNewAttempt[0].definition, ShouldResembleProto, execState.GetRequirement().GetDefinitions()[0])
						So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY),
						})
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
						So(executor.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
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
						})
					})
					Convey("Critical and can NOT retry", func() {
						// Quota doesn't allow retrying permanent failure.
						tj = ensureTryjob(ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeTrue)
						So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY),
						})
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_FAILED)
						So(execState.Failures, ShouldResembleProto, &tryjob.ExecutionState_Failures{
							UnsuccessfulResults: []*tryjob.ExecutionState_Failures_UnsuccessfulResult{
								{TryjobId: tjID},
							},
						})
						So(executor.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
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
						})
					})
					Convey("Tryjob is not critical", func() {
						tj = ensureTryjob(ctx, tjID, def, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)
						execState.GetRequirement().GetDefinitions()[0].Critical = false
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeTrue)
						So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY),
						})
						// Still consider execution as succeeded even though non-critical
						// tryjob has failed.
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_SUCCEEDED)
						So(executor.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
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
						})
					})
				})

				Convey("Untriggered", func() {
					ensureTryjob(ctx, tjID, def, tryjob.Status_UNTRIGGERED, tryjob.Result_UNKNOWN)
					Convey("Reused", func() {
						tryjob.LatestAttempt(execState.GetExecutions()[0]).Reused = true
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeFalse)
						So(plan.triggerNewAttempt, ShouldHaveLength, 1)
						So(plan.triggerNewAttempt[0].definition, ShouldResembleProto, execState.GetRequirement().GetDefinitions()[0])
						attempt := makeAttempt(tjID, tryjob.Status_UNTRIGGERED, tryjob.Result_UNKNOWN)
						attempt.Reused = true
						So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{attempt})
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
					})
					Convey("Not critical", func() {
						execState.GetRequirement().GetDefinitions()[0].Critical = false
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeTrue)
					})
					So(executor.logEntries, ShouldBeEmpty)
				})
			})

			Convey("Multiple Tryjobs", func() {
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

				Convey("Has non-ended critical Tryjobs", func() {
					ensureTryjob(ctx, 101, builder1Def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					ensureTryjob(ctx, 201, builder2Def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					// 301 has not completed.
					plan := prepPlan(execState, 101, 201)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
				})
				Convey("Execution ends when all critical tryjob ended", func() {
					ensureTryjob(ctx, 101, builder1Def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					// 201 (non-critical) has not completed.
					ensureTryjob(ctx, 301, builder3Def, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					plan := prepPlan(execState, 101, 301)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_SUCCEEDED)
				})
				Convey("Failed critical Tryjob fails the execution", func() {
					ensureTryjob(ctx, 101, builder1Def, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)
					// One critical tryjob is still running.
					ensureTryjob(ctx, 301, builder3Def, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
					plan := prepPlan(execState, 101, 301)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_FAILED)
					So(execState.Failures, ShouldResembleProto, &tryjob.ExecutionState_Failures{
						UnsuccessfulResults: []*tryjob.ExecutionState_Failures_UnsuccessfulResult{
							{TryjobId: 101},
						},
					})
				})
				Convey("Can retry multiple", func() {
					ensureTryjob(ctx, 101, builder1Def, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					// 201 has not completed.
					ensureTryjob(ctx, 301, builder3Def, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					plan := prepPlan(execState, 101, 301)
					So(plan.isEmpty(), ShouldBeFalse)
					So(plan.triggerNewAttempt, ShouldHaveLength, 2)
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
				})
			})
		})

		Convey("Requirement Changed", func() {
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

			Convey("No-op if current version newer than target version", func() {
				execState := newExecStateBuilder().
					withRequirementVersion(int(r.Tryjobs.GetRequirementVersion()) + 1).
					build()
				_, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(plan.isEmpty(), ShouldBeTrue)
				So(executor.logEntries, ShouldBeEmpty)
			})

			Convey("New Requirement", func() {
				execState := newExecStateBuilder().build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(execState.Requirement, ShouldResembleProto, latestReqmt)
				So(plan.triggerNewAttempt, ShouldHaveLength, 1)
				So(plan.triggerNewAttempt[0].definition, ShouldResembleProto, builderFooDef)
				So(executor.logEntries, ShouldBeEmpty)
			})

			Convey("Removed Tryjob", func() {
				removedDef := makeDefinition("removed", true)
				execState := newExecStateBuilder().
					appendDefinition(removedDef).
					appendDefinition(builderFooDef).
					build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(execState.Requirement, ShouldResembleProto, latestReqmt)
				So(plan.triggerNewAttempt, ShouldBeEmpty)
				So(plan.discard, ShouldHaveLength, 1)
				So(plan.discard[0].definition, ShouldResembleProto, removedDef)
				So(execState.Executions, ShouldHaveLength, 1)
				So(plan.discard[0].discardReason, ShouldEqual, noLongerRequiredInConfig)
				So(executor.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
					{
						Time: timestamppb.New(ct.Clock.Now().UTC()),
						Kind: &tryjob.ExecutionLogEntry_RequirementChanged_{
							RequirementChanged: &tryjob.ExecutionLogEntry_RequirementChanged{},
						},
					},
				})
			})

			Convey("Changed Tryjob", func() {
				builderFooOriginal := proto.Clone(builderFooDef).(*tryjob.Definition)
				builderFooOriginal.DisableReuse = !builderFooDef.DisableReuse
				execState := newExecStateBuilder().
					appendDefinition(builderFooOriginal).
					build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(execState.Requirement, ShouldResembleProto, latestReqmt)
				So(plan.isEmpty(), ShouldBeTrue)
				So(executor.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
					{
						Time: timestamppb.New(ct.Clock.Now().UTC()),
						Kind: &tryjob.ExecutionLogEntry_RequirementChanged_{
							RequirementChanged: &tryjob.ExecutionLogEntry_RequirementChanged{},
						},
					},
				})
			})

			Convey("Change in retry config", func() {
				execState := newExecStateBuilder().
					appendDefinition(builderFooDef).
					withRetryConfig(&cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 10,
					}).
					build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(execState.Requirement, ShouldResembleProto, latestReqmt)
				So(plan.isEmpty(), ShouldBeTrue)
				So(executor.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
					{
						Time: timestamppb.New(ct.Clock.Now().UTC()),
						Kind: &tryjob.ExecutionLogEntry_RequirementChanged_{
							RequirementChanged: &tryjob.ExecutionLogEntry_RequirementChanged{},
						},
					},
				})
			})

			Convey("Empty definitions", func() {
				latestReqmt.Definitions = nil
				execState := newExecStateBuilder().
					appendDefinition(builderFooDef).
					build()
				execState, plan, err := executor.prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(execState.Requirement, ShouldResembleProto, latestReqmt)
				So(plan.discard, ShouldHaveLength, 1)
				So(execState.Status, ShouldEqual, tryjob.ExecutionState_SUCCEEDED)
			})
		})
	})
}

func TestExecutePlan(t *testing.T) {
	t.Parallel()
	Convey("ExecutePlan", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		Convey("Discard tryjobs", func() {
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
			So(err, ShouldBeNil)
			So(executor.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
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
			})
		})

		Convey("Trigger new attempt", func() {
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
			So(datastore.Put(ctx, runCL), ShouldBeNil)
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

			Convey("New Tryjob is not launched when tryjob can be reused", func() {
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
				reuseTryjob := &tryjob.Tryjob{
					ExternalID:       tryjob.MustBuildbucketID(bbHost, buildID),
					EVersion:         1,
					EntityCreateTime: now.Add(-staleTryjobAge / 2),
					EntityUpdateTime: now.Add(-1 * time.Minute),
					ReuseKey:         computeReuseKey([]*run.RunCL{runCL}),
					CLPatchsets:      tryjob.CLPatchsets{tryjob.MakeCLPatchset(runCL.ID, gPatchset)},
					Definition:       def,
					Status:           tryjob.Status_ENDED,
					LaunchedBy:       common.MakeRunID(lProject, now.Add(-2*time.Hour), 1, []byte("efgh")),
					Result:           result,
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{reuseTryjob}, nil)
				}, nil), ShouldBeNil)
				execState, err := executor.executePlan(ctx, &plan{
					triggerNewAttempt: []planItem{
						{
							definition: def,
							execution:  execution,
						},
					},
				}, r, execState)
				So(err, ShouldBeNil)
				So(execState.GetStatus(), ShouldEqual, tryjob.ExecutionState_RUNNING)
				So(execState.GetExecutions()[0].GetAttempts(), ShouldResembleProto,
					[]*tryjob.ExecutionState_Execution_Attempt{
						{
							TryjobId:   int64(reuseTryjob.ID),
							ExternalId: string(tryjob.MustBuildbucketID(bbHost, buildID)),
							Status:     tryjob.Status_ENDED,
							Result:     result,
							Reused:     true,
						},
					})
				So(executor.stagedMetricReportFns, ShouldBeEmpty)
			})

			Convey("When Tryjob can't be reused, thus launch a new Tryjob", func() {
				execState, err := executor.executePlan(ctx, &plan{
					triggerNewAttempt: []planItem{
						{
							definition: def,
							execution:  execution,
						},
					},
				}, r, execState)
				So(err, ShouldBeNil)
				So(execState.GetStatus(), ShouldEqual, tryjob.ExecutionState_RUNNING)
				So(execState.GetExecutions()[0].GetAttempts(), ShouldHaveLength, 1)
				attempt := execState.GetExecutions()[0].GetAttempts()[0]
				So(attempt.GetTryjobId(), ShouldNotEqual, 0)
				So(attempt.GetExternalId(), ShouldNotBeEmpty)
				So(attempt.GetStatus(), ShouldEqual, tryjob.Status_TRIGGERED)
				So(attempt.GetReused(), ShouldBeFalse)
				So(executor.stagedMetricReportFns, ShouldHaveLength, 1)
				executor.stagedMetricReportFns[0](ctx)
				tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, def, func(ctx context.Context) {
					So(ct.TSMonSentValue(ctx, metrics.Public.TryjobLaunched, lProject, configGroupName, true, false), ShouldEqual, 1)
				})
			})

			Convey("Fail the execution if encounter launch failure", func() {
				def.GetBuildbucket().GetBuilder().Project = "another_proj"
				execState, err := executor.executePlan(ctx, &plan{
					triggerNewAttempt: []planItem{
						{
							definition: def,
							execution:  execution,
						},
					},
				}, r, execState)
				So(err, ShouldBeNil)
				So(execState.GetExecutions()[0].GetAttempts(), ShouldHaveLength, 1)
				attempt := execState.GetExecutions()[0].GetAttempts()[0]
				So(attempt.GetTryjobId(), ShouldNotEqual, 0)
				So(attempt.GetExternalId(), ShouldBeEmpty)
				So(attempt.GetStatus(), ShouldEqual, tryjob.Status_UNTRIGGERED)
				So(execState.Status, ShouldEqual, tryjob.ExecutionState_FAILED)
				So(execState.Failures, ShouldResembleProto, &tryjob.ExecutionState_Failures{
					LaunchFailures: []*tryjob.ExecutionState_Failures_LaunchFailure{
						{
							Definition: def,
							Reason:     "received NotFound from buildbucket. message: builder another_proj/some_bucket/some_builder not found",
						},
					},
				})
				So(executor.stagedMetricReportFns, ShouldBeEmpty)
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

func ensureTryjob(ctx context.Context, id int64, def *tryjob.Definition, status tryjob.Status, resultStatus tryjob.Result_Status) *tryjob.Tryjob {
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
	So(datastore.Put(ctx, tj), ShouldBeNil)
	return tj
}
