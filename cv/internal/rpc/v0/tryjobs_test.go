// Copyright 2023 The LUCI Authors.
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

package rpc

import (
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestMakeTryjobInvocations(t *testing.T) {
	t.Parallel()

	ftt.Run("MakeTryjobInvocations", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const lProject = "infra"
		const bbHost = "buildbucket.example.com"
		r := &run.Run{
			ID: common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef")),
		}

		builderFoo := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "foo",
		}
		builderFooEquivalent := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "foo-equi",
		}
		builderBar := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "bar",
		}
		cg := &cfgpb.ConfigGroup{
			Name: "main",
			Verifiers: &cfgpb.Verifiers{
				Tryjob: &cfgpb.Verifiers_Tryjob{
					Builders: []*cfgpb.Verifiers_Tryjob_Builder{
						{
							Name: bbutil.FormatBuilderID(builderFoo),
							EquivalentTo: &cfgpb.Verifiers_Tryjob_EquivalentBuilder{
								Name: bbutil.FormatBuilderID(builderFooEquivalent),
							},
						},
						{
							Name: bbutil.FormatBuilderID(builderBar),
						},
					},
				},
			},
		}

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{cg},
		})
		r.ConfigGroupID = prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0]

		t.Run("comprehensive", func(t *ftt.Test) {
			r.Tryjobs = &run.Tryjobs{
				State: &tryjob.ExecutionState{
					Requirement: &tryjob.Requirement{
						Definitions: []*tryjob.Definition{
							{
								Backend: &tryjob.Definition_Buildbucket_{
									Buildbucket: &tryjob.Definition_Buildbucket{
										Host:    bbHost,
										Builder: builderFoo,
									},
								},
								Critical: true,
							},
							{
								Backend: &tryjob.Definition_Buildbucket_{
									Buildbucket: &tryjob.Definition_Buildbucket{
										Host:    bbHost,
										Builder: builderBar,
									},
								},
							},
						},
					},
					Executions: []*tryjob.ExecutionState_Execution{
						{
							Attempts: []*tryjob.ExecutionState_Execution_Attempt{
								{
									Status:     tryjob.Status_ENDED,
									ExternalId: string(tryjob.MustBuildbucketID(bbHost, 50000)),
									Result: &tryjob.Result{
										Status: tryjob.Result_FAILED_TRANSIENTLY,
										Backend: &tryjob.Result_Buildbucket_{
											Buildbucket: &tryjob.Result_Buildbucket{
												Id:      50000,
												Builder: builderFoo,
											},
										},
									},
									Reused: true,
								},
								{
									Status:     tryjob.Status_ENDED,
									ExternalId: string(tryjob.MustBuildbucketID(bbHost, 49999)),
									Result: &tryjob.Result{
										Status: tryjob.Result_SUCCEEDED,
										Backend: &tryjob.Result_Buildbucket_{
											Buildbucket: &tryjob.Result_Buildbucket{
												Id:      49999,
												Builder: builderFoo,
											},
										},
									},
								},
							},
						},
						{
							Attempts: []*tryjob.ExecutionState_Execution_Attempt{
								{
									Status:     tryjob.Status_TRIGGERED,
									ExternalId: string(tryjob.MustBuildbucketID(bbHost, 60000)),
									Result: &tryjob.Result{
										Status: tryjob.Result_UNKNOWN,
										Backend: &tryjob.Result_Buildbucket_{
											Buildbucket: &tryjob.Result_Buildbucket{
												Id:      60000,
												Builder: builderBar,
											},
										},
									},
								},
							},
						},
					},
				},
			}
			ti, err := makeTryjobInvocations(ctx, r)
			assert.NoErr(t, err)
			assert.Loosely(t, ti, should.Resemble([]*apiv0pb.TryjobInvocation{
				{
					BuilderConfig: cg.GetVerifiers().GetTryjob().GetBuilders()[0],
					Status:        apiv0pb.TryjobStatus_SUCCEEDED,
					Critical:      true,
					Attempts: []*apiv0pb.TryjobInvocation_Attempt{
						{
							Status: apiv0pb.TryjobStatus_SUCCEEDED,
							Result: &apiv0pb.TryjobResult{
								Backend: &apiv0pb.TryjobResult_Buildbucket_{
									Buildbucket: &apiv0pb.TryjobResult_Buildbucket{
										Host:    bbHost,
										Id:      49999,
										Builder: builderFoo,
									},
								},
							},
							Reuse: false,
						},
						{
							Status: apiv0pb.TryjobStatus_FAILED,
							Result: &apiv0pb.TryjobResult{
								Backend: &apiv0pb.TryjobResult_Buildbucket_{
									Buildbucket: &apiv0pb.TryjobResult_Buildbucket{
										Host:    bbHost,
										Id:      50000,
										Builder: builderFoo,
									},
								},
							},
							Reuse: true,
						},
					},
				},
				{
					BuilderConfig: cg.GetVerifiers().GetTryjob().GetBuilders()[1],
					Status:        apiv0pb.TryjobStatus_RUNNING,
					Critical:      false,
					Attempts: []*apiv0pb.TryjobInvocation_Attempt{
						{
							Status: apiv0pb.TryjobStatus_RUNNING,
							Result: &apiv0pb.TryjobResult{
								Backend: &apiv0pb.TryjobResult_Buildbucket_{
									Buildbucket: &apiv0pb.TryjobResult_Buildbucket{
										Host:    bbHost,
										Id:      60000,
										Builder: builderBar,
									},
								},
							},
						},
					},
				},
			}))
		})

		t.Run("Skip if builder is not seen in the config", func(t *ftt.Test) {
			r.Tryjobs = &run.Tryjobs{
				State: &tryjob.ExecutionState{
					Requirement: &tryjob.Requirement{
						Definitions: []*tryjob.Definition{
							{
								Backend: &tryjob.Definition_Buildbucket_{
									Buildbucket: &tryjob.Definition_Buildbucket{
										Host: bbHost,
										Builder: &bbpb.BuilderID{
											Project: lProject,
											Bucket:  "try",
											Builder: "undefined",
										},
									},
								},
								Critical: true,
							},
						},
					},
					Executions: []*tryjob.ExecutionState_Execution{
						{
							Attempts: []*tryjob.ExecutionState_Execution_Attempt{
								{
									Status: tryjob.Status_PENDING,
									Result: &tryjob.Result{
										Status: tryjob.Result_UNKNOWN,
									},
								},
							},
						},
					},
				},
			}
			ti, err := makeTryjobInvocations(ctx, r)
			assert.NoErr(t, err)
			assert.Loosely(t, ti, should.BeEmpty)
		})

		t.Run("Omit empty attempt", func(t *ftt.Test) {
			r.Tryjobs = &run.Tryjobs{
				State: &tryjob.ExecutionState{
					Requirement: &tryjob.Requirement{
						Definitions: []*tryjob.Definition{
							{
								Backend: &tryjob.Definition_Buildbucket_{
									Buildbucket: &tryjob.Definition_Buildbucket{
										Host:    bbHost,
										Builder: builderFoo,
									},
								},
								Critical: true,
							},
						},
					},
					Executions: []*tryjob.ExecutionState_Execution{
						{
							Attempts: nil,
						},
					},
				},
			}
			ti, err := makeTryjobInvocations(ctx, r)
			assert.NoErr(t, err)
			assert.Loosely(t, ti, should.BeEmpty)
		})
	})
}
