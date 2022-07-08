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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPrepExecutionPlan(t *testing.T) {
	t.Parallel()
	Convey("PrepExecutionPlan", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		Convey("Tryjobs Updated", func() {
			const builderFoo = "foo"
			prepPlan := func(execState *tryjob.ExecutionState, updatedTryjobs ...int64) *plan {
				p, err := prepExecutionPlan(ctx, execState, nil, updatedTryjobs, false)
				So(err, ShouldBeNil)
				return p
			}
			Convey("Ignore update", func() {
				const prevTryjobID = 101
				const curTryjobID = 102
				execState := newExecStateBuilder().
					appendDefinition(makeDefinition(builderFoo, true)).
					appendAttempt(builderFoo, makeAttempt(prevTryjobID, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)).
					appendAttempt(builderFoo, makeAttempt(curTryjobID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)).
					build()
				original := proto.Clone(execState).(*tryjob.ExecutionState)
				Convey("For previous attempts", func() {
					ensureTryjob(ctx, prevTryjobID, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					plan := prepPlan(execState, prevTryjobID)
					So(plan.isEmpty(), ShouldBeTrue)
				})
				Convey("For not relevant Tryjob", func() {
					const randomTryjob int64 = 567
					ensureTryjob(ctx, randomTryjob, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					plan := prepPlan(execState, randomTryjob)
					So(plan.isEmpty(), ShouldBeTrue)
				})
				So(execState, ShouldResembleProto, original)
			})

			Convey("Single Tryjob", func() {
				const tjID = 101
				execState := newExecStateBuilder().
					appendDefinition(makeDefinition(builderFoo, true)).
					appendAttempt(builderFoo, makeAttempt(tjID, tryjob.Status_PENDING, tryjob.Result_UNKNOWN)).
					withRetryConfig(&cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota:            2,
						GlobalQuota:            10,
						FailureWeight:          5,
						TransientFailureWeight: 1,
					}).
					build()
				Convey("Succeeded", func() {
					ensureTryjob(ctx, tjID, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					plan := prepPlan(execState, tjID)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
						makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED),
					})
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_SUCCEEDED)
				})

				Convey("Still Running", func() {
					ensureTryjob(ctx, tjID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
					Convey("Tryjob is critical", func() {
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeTrue)
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
					})
					Convey("Tryjob is not critical", func() {
						execState.GetRequirement().GetDefinitions()[0].Critical = false
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeTrue)
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_SUCCEEDED)
					})
					So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
						makeAttempt(tjID, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN),
					})
				})

				Convey("Failed", func() {
					Convey("Critical and can retry", func() {
						// Quota allows retrying transient failure.
						ensureTryjob(ctx, tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeFalse)
						So(plan.triggerNewAttempt, ShouldHaveLength, 1)
						So(plan.triggerNewAttempt[0].definition, ShouldResembleProto, execState.GetRequirement().GetDefinitions()[0])
						So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY),
						})
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
					})
					Convey("Critical and can NOT retry", func() {
						// Quota doesn't allow retrying permanent failure
						ensureTryjob(ctx, tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeTrue)
						So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY),
						})
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_FAILED)
						So(execState.FailureReason, ShouldContainSubstring, "Failed Tryjobs")
					})
					Convey("Tryjob is not critical", func() {
						ensureTryjob(ctx, tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)
						execState.GetRequirement().GetDefinitions()[0].Critical = false
						plan := prepPlan(execState, tjID)
						So(plan.isEmpty(), ShouldBeTrue)
						So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
							makeAttempt(tjID, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY),
						})
						// Still consider execution as succeeded even though non-critical
						// tryjob has failed.
						So(execState.Status, ShouldEqual, tryjob.ExecutionState_SUCCEEDED)
					})
				})

				Convey("Untriggered", func() {
					ensureTryjob(ctx, tjID, tryjob.Status_UNTRIGGERED, tryjob.Result_UNKNOWN)
					plan := prepPlan(execState, tjID)
					So(plan.isEmpty(), ShouldBeFalse)
					So(plan.triggerNewAttempt, ShouldHaveLength, 1)
					So(plan.triggerNewAttempt[0].definition, ShouldResembleProto, execState.GetRequirement().GetDefinitions()[0])
					So(execState.Executions[0].Attempts, ShouldResembleProto, []*tryjob.ExecutionState_Execution_Attempt{
						makeAttempt(tjID, tryjob.Status_UNTRIGGERED, tryjob.Result_UNKNOWN),
					})
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
				})
			})

			Convey("Multiple Tryjobs", func() {
				execState := newExecStateBuilder().
					appendDefinition(makeDefinition("builder1", true)).
					appendAttempt("builder1", makeAttempt(101, tryjob.Status_PENDING, tryjob.Result_UNKNOWN)).
					appendDefinition(makeDefinition("builder2", false)).
					appendAttempt("builder2", makeAttempt(201, tryjob.Status_PENDING, tryjob.Result_UNKNOWN)).
					appendDefinition(makeDefinition("builder3", true)).
					appendAttempt("builder3", makeAttempt(301, tryjob.Status_PENDING, tryjob.Result_UNKNOWN)).
					withRetryConfig(&cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota:            2,
						GlobalQuota:            10,
						FailureWeight:          5,
						TransientFailureWeight: 1,
					}).
					build()

				Convey("Has non ended critical tryjobs", func() {
					ensureTryjob(ctx, 101, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					ensureTryjob(ctx, 201, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					// 301 has not completed
					plan := prepPlan(execState, 101, 201)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_RUNNING)
				})
				Convey("Execution ends when all critical tryjob ended", func() {
					ensureTryjob(ctx, 101, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					// 201 (non-critical) has not completed
					ensureTryjob(ctx, 301, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)
					plan := prepPlan(execState, 101, 301)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_SUCCEEDED)
				})
				Convey("Failed critical Tryjob fails the execution", func() {
					ensureTryjob(ctx, 101, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)
					// one critical tryjob is still running.
					ensureTryjob(ctx, 301, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
					plan := prepPlan(execState, 101, 301)
					So(plan.isEmpty(), ShouldBeTrue)
					So(execState.Status, ShouldEqual, tryjob.ExecutionState_FAILED)
					So(execState.FailureReason, ShouldContainSubstring, "Failed Tryjobs")
				})
				Convey("Can retry multiple", func() {
					ensureTryjob(ctx, 101, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
					// 201 has not completed
					ensureTryjob(ctx, 301, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)
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
					Requirement:          latestReqmt,
					RequirementVersion:   2,
					RequirementComputeAt: timestamppb.New(ct.Clock.Now()),
				},
			}
			Convey("Noop if execution has ended already", func() {
				for _, st := range []tryjob.ExecutionState_Status{
					tryjob.ExecutionState_SUCCEEDED,
					tryjob.ExecutionState_FAILED} {
					execState := newExecStateBuilder().
						withStatus(st).
						build()
					plan, err := prepExecutionPlan(ctx, execState, r, nil, true)
					So(err, ShouldBeNil)
					So(plan.isEmpty(), ShouldBeTrue)
				}
			})

			Convey("Noop if current version newer than target version", func() {
				execState := newExecStateBuilder().
					withRequirementVersion(int(r.Tryjobs.GetRequirementVersion()) + 1).
					build()
				plan, err := prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(plan.isEmpty(), ShouldBeTrue)
			})

			Convey("New Tryjob", func() {
				execState := newExecStateBuilder().build()
				plan, err := prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(execState.Requirement, ShouldResembleProto, latestReqmt)
				So(plan.triggerNewAttempt, ShouldHaveLength, 1)
				So(plan.triggerNewAttempt[0].definition, ShouldResembleProto, builderFooDef)
			})

			Convey("Removed Tryjob", func() {
				removedDef := makeDefinition("removed", true)
				execState := newExecStateBuilder().
					appendDefinition(removedDef).
					appendDefinition(builderFooDef).
					build()
				plan, err := prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(execState.Requirement, ShouldResembleProto, latestReqmt)
				So(plan.triggerNewAttempt, ShouldBeEmpty)
				So(plan.discard, ShouldHaveLength, 1)
				So(plan.discard[0].definition, ShouldResembleProto, removedDef)
				So(execState.Executions, ShouldHaveLength, 1)
				So(plan.discard[0].discardReason, ShouldEqual, noLongerRequiredInConfig)
			})

			Convey("Changed Tryjob", func() {
				builderFooOriginal := proto.Clone(builderFooDef).(*tryjob.Definition)
				builderFooOriginal.DisableReuse = !builderFooDef.DisableReuse
				execState := newExecStateBuilder().
					appendDefinition(builderFooOriginal).
					build()
				plan, err := prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(execState.Requirement, ShouldResembleProto, latestReqmt)
				So(plan.isEmpty(), ShouldBeTrue)
			})

			Convey("No change", func() {
				execState := newExecStateBuilder().
					appendDefinition(builderFooDef).
					build()
				plan, err := prepExecutionPlan(ctx, execState, r, nil, true)
				So(err, ShouldBeNil)
				So(execState.Requirement, ShouldResembleProto, latestReqmt)
				So(plan.isEmpty(), ShouldBeTrue)
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

func ensureTryjob(ctx context.Context, id int64, status tryjob.Status, resultStatus tryjob.Result_Status) *tryjob.Tryjob {
	tj := &tryjob.Tryjob{
		ID:         common.TryjobID(id),
		ExternalID: tryjob.MustBuildbucketID("buildbucket.example.com", math.MaxInt64-id),
		Status:     status,
		Result: &tryjob.Result{
			Status: resultStatus,
		},
	}
	So(datastore.Put(ctx, tj), ShouldBeNil)
	return tj
}
