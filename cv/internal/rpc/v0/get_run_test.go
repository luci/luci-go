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

package rpc

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestGetRun(t *testing.T) {
	t.Parallel()

	ftt.Run("GetRun", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		rs := RunsServer{}

		const lProject = "infra"
		rid := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))
		builderFoo := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "foo",
		}
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			// TODO(crbug/1233963): remove once non-legacy ACLs are implemented.
			CqStatusHost: "chromium-cq-status.appspot.com",
			ConfigGroups: []*cfgpb.ConfigGroup{{
				Name: "first",
				Verifiers: &cfgpb.Verifiers{
					Tryjob: &cfgpb.Verifiers_Tryjob{
						Builders: []*cfgpb.Verifiers_Tryjob_Builder{
							{
								Name: protoutil.FormatBuilderID(builderFoo),
							},
						},
					},
				},
			}},
		})

		configGroupID := prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0]

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{acls.V0APIAllowGroup},
		})

		saveAndGet := func(r *run.Run) *apiv0pb.Run {
			assert.NoErr(t, datastore.Put(ctx, r))
			resp, err := rs.GetRun(ctx, &apiv0pb.GetRunRequest{Id: rid.PublicID()})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.OK))
			return resp
		}

		t.Run("w/o access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := rs.GetRun(ctx, &apiv0pb.GetRunRequest{Id: rid.PublicID()})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
		})

		t.Run("w/ an invalid public ID", func(t *ftt.Test) {
			_, err := rs.GetRun(ctx, &apiv0pb.GetRunRequest{Id: "something valid"})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
		})

		t.Run("w/ an non-existing Run ID", func(t *ftt.Test) {
			_, err := rs.GetRun(ctx, &apiv0pb.GetRunRequest{Id: rid.PublicID()})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
		})

		t.Run("w/ an existing RunID", func(t *ftt.Test) {
			const gHost = "r-review.example.com"
			cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
			cl2 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)
			cl3 := changelist.MustGobID(gHost, 3).MustCreateIfNotExists(ctx)
			epoch := testclock.TestRecentTimeUTC.Truncate(time.Millisecond)
			r := &run.Run{
				ID:            rid,
				Status:        run.Status_SUCCEEDED,
				ConfigGroupID: configGroupID,
				CreateTime:    epoch,
				StartTime:     epoch.Add(time.Second),
				UpdateTime:    epoch.Add(time.Minute),
				EndTime:       epoch.Add(time.Hour),
				Owner:         "user:foo@example.org",
				CreatedBy:     "user:bar@example.org",
				BilledTo:      "user:bar@example.org",
				CLs:           common.MakeCLIDs(int64(cl1.ID), int64(cl2.ID), int64(cl3.ID)),
			}
			assert.Loosely(t, datastore.Put(
				ctx,
				&run.RunCL{
					Run: datastore.MakeKey(ctx, common.RunKind, string(rid)),
					ID:  cl1.ID, IndexedID: cl1.ID,
					ExternalID: cl1.ExternalID,
					Detail: &changelist.Snapshot{
						Patchset: 39,
					},
				},
				&run.RunCL{
					Run: datastore.MakeKey(ctx, common.RunKind, string(rid)),
					ID:  cl2.ID, IndexedID: cl2.ID,
					ExternalID: cl2.ExternalID,
					Detail: &changelist.Snapshot{
						Patchset: 40,
					},
				},
				&run.RunCL{
					Run: datastore.MakeKey(ctx, common.RunKind, string(rid)),
					ID:  cl3.ID, IndexedID: cl3.ID,
					ExternalID: cl3.ExternalID,
					Detail: &changelist.Snapshot{
						Patchset: 41,
					},
				},
			), should.BeNil)

			assert.That(t, saveAndGet(r), should.Match(&apiv0pb.Run{
				Id:         rid.PublicID(),
				Status:     apiv0pb.Run_SUCCEEDED,
				CreateTime: timestamppb.New(epoch),
				StartTime:  timestamppb.New(epoch.Add(time.Second)),
				UpdateTime: timestamppb.New(epoch.Add(time.Minute)),
				EndTime:    timestamppb.New(epoch.Add(time.Hour)),
				Owner:      "user:foo@example.org",
				CreatedBy:  "user:bar@example.org",
				BilledTo:   "user:bar@example.org",
				Cls: []*apiv0pb.GerritChange{
					{Host: gHost, Change: int64(cl1.ID), Patchset: 39},
					{Host: gHost, Change: int64(cl2.ID), Patchset: 40},
					{Host: gHost, Change: int64(cl3.ID), Patchset: 41},
				},
			}))

			t.Run("w/ tryjobs", func(t *ftt.Test) {
				r.Tryjobs = &run.Tryjobs{
					State: &tryjob.ExecutionState{
						Requirement: &tryjob.Requirement{
							Definitions: []*tryjob.Definition{
								{
									Backend: &tryjob.Definition_Buildbucket_{
										Buildbucket: &tryjob.Definition_Buildbucket{
											Host:    "bb.example.com",
											Builder: builderFoo,
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
										TryjobId:   1,
										ExternalId: string(tryjob.MustBuildbucketID("bb.example.com", 11)),
										Status:     tryjob.Status_ENDED,
										Result: &tryjob.Result{
											Status: tryjob.Result_SUCCEEDED,
											Backend: &tryjob.Result_Buildbucket_{
												Buildbucket: &tryjob.Result_Buildbucket{
													Id:      11,
													Status:  bbpb.Status_SUCCESS,
													Builder: builderFoo,
												},
											},
										},
										Reused: true,
									},
								},
							},
						},
					},
				}
				assert.That(t, saveAndGet(r).TryjobInvocations, should.Match([]*apiv0pb.TryjobInvocation{
					{
						BuilderConfig: &cfgpb.Verifiers_Tryjob_Builder{
							Name: protoutil.FormatBuilderID(builderFoo),
						},
						Status:   apiv0pb.TryjobStatus_SUCCEEDED,
						Critical: true,
						Attempts: []*apiv0pb.TryjobInvocation_Attempt{
							{
								Status: apiv0pb.TryjobStatus_SUCCEEDED,
								Result: &apiv0pb.TryjobResult{
									Backend: &apiv0pb.TryjobResult_Buildbucket_{
										Buildbucket: &apiv0pb.TryjobResult_Buildbucket{
											Host:    "bb.example.com",
											Id:      11,
											Builder: builderFoo,
										},
									},
								},
								Reuse: true,
							},
						},
					},
				}))
			})

			t.Run("w/ submission", func(t *ftt.Test) {
				r.Submission = &run.Submission{
					SubmittedCls: []int64{int64(cl1.ID)},
					FailedCls:    []int64{int64(cl3.ID)},
				}
				assert.That(t, saveAndGet(r).Submission, should.Match(&apiv0pb.Run_Submission{
					SubmittedClIndexes: []int32{0},
					FailedClIndexes:    []int32{2},
				}))
			})
		})
	})
}
