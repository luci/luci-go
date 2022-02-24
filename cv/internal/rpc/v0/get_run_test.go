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
	"go.chromium.org/luci/common/clock/testclock"
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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetRun(t *testing.T) {
	t.Parallel()

	Convey("GetRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		rs := RunsServer{}

		rid := common.RunID("prj/123-deadbeef")
		prjcfgtest.Create(ctx, "prj", &cfgpb.Config{
			// TODO(crbug/1233963): remove once non-legacy ACLs are implemented.
			CqStatusHost: "chromium-cq-status.appspot.com",
			ConfigGroups: []*cfgpb.ConfigGroup{{
				Name: "first",
			}},
		})

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{acls.V0APIAllowGroup},
		})

		saveAndGet := func(r *run.Run) *apiv0pb.Run {
			So(datastore.Put(ctx, r), ShouldBeNil)
			resp, err := rs.GetRun(ctx, &apiv0pb.GetRunRequest{Id: rid.PublicID()})
			So(grpcutil.Code(err), ShouldEqual, codes.OK)
			return resp
		}

		Convey("w/o access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := rs.GetRun(ctx, &apiv0pb.GetRunRequest{Id: rid.PublicID()})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("w/ an invalid public ID", func() {
			_, err := rs.GetRun(ctx, &apiv0pb.GetRunRequest{Id: "something valid"})
			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
		})

		Convey("w/ an non-existing Run ID", func() {
			_, err := rs.GetRun(ctx, &apiv0pb.GetRunRequest{Id: rid.PublicID()})
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("w/ an existing RunID", func() {
			const gHost = "r-review.example.com"
			cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
			cl2 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)
			cl3 := changelist.MustGobID(gHost, 3).MustCreateIfNotExists(ctx)
			epoch := testclock.TestRecentTimeUTC.Truncate(time.Millisecond)
			r := &run.Run{
				ID:         rid,
				Status:     run.Status_SUCCEEDED,
				CreateTime: epoch,
				StartTime:  epoch.Add(time.Second),
				UpdateTime: epoch.Add(time.Minute),
				EndTime:    epoch.Add(time.Hour),
				Owner:      "user:foo@example.org",
				CLs:        common.MakeCLIDs(int64(cl1.ID), int64(cl2.ID), int64(cl3.ID)),
			}
			So(datastore.Put(
				ctx,
				&run.RunCL{
					Run: datastore.MakeKey(ctx, run.RunKind, string(rid)),
					ID:  cl1.ID, IndexedID: cl1.ID,
					ExternalID: cl1.ExternalID,
					Detail: &changelist.Snapshot{
						Patchset: 39,
					},
				},
				&run.RunCL{
					Run: datastore.MakeKey(ctx, run.RunKind, string(rid)),
					ID:  cl2.ID, IndexedID: cl2.ID,
					ExternalID: cl2.ExternalID,
					Detail: &changelist.Snapshot{
						Patchset: 40,
					},
				},
				&run.RunCL{
					Run: datastore.MakeKey(ctx, run.RunKind, string(rid)),
					ID:  cl3.ID, IndexedID: cl3.ID,
					ExternalID: cl3.ExternalID,
					Detail: &changelist.Snapshot{
						Patchset: 41,
					},
				},
			), ShouldBeNil)

			So(saveAndGet(r), ShouldResembleProto, &apiv0pb.Run{
				Id:         rid.PublicID(),
				Status:     apiv0pb.Run_SUCCEEDED,
				CreateTime: timestamppb.New(epoch),
				StartTime:  timestamppb.New(epoch.Add(time.Second)),
				UpdateTime: timestamppb.New(epoch.Add(time.Minute)),
				EndTime:    timestamppb.New(epoch.Add(time.Hour)),
				Owner:      "user:foo@example.org",
				Cls: []*apiv0pb.GerritChange{
					{Host: gHost, Change: int64(cl1.ID), Patchset: 39},
					{Host: gHost, Change: int64(cl2.ID), Patchset: 40},
					{Host: gHost, Change: int64(cl3.ID), Patchset: 41},
				},
			})

			Convey("w/ tryjobs", func() {
				def := &tryjob.Definition{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "bb",
							Builder: &bbpb.BuilderID{
								Project: "prj",
								Bucket:  "ci",
								Builder: "foo",
							},
						},
					},
				}
				r.Tryjobs = &run.Tryjobs{Tryjobs: []*run.Tryjob{
					// first tryjob was cancelled.
					{
						Id:         1,
						ExternalId: string(tryjob.MustBuildbucketID("bb-host", 11)),
						Definition: def,
						Status:     tryjob.Status_CANCELLED,
						Result: &tryjob.Result{
							Status: tryjob.Result_FAILED_TRANSIENTLY,
							Backend: &tryjob.Result_Buildbucket_{
								Buildbucket: &tryjob.Result_Buildbucket{
									Id:     11,
									Status: bbpb.Status_CANCELED,
								},
							},
						},
					},
					// but the next one was succeeded.
					{
						Id:         2,
						ExternalId: string(tryjob.MustBuildbucketID("bb-host", 12)),
						Definition: def,
						Status:     tryjob.Status_ENDED,
						Result: &tryjob.Result{
							Status: tryjob.Result_SUCCEEDED,
							Backend: &tryjob.Result_Buildbucket_{
								Buildbucket: &tryjob.Result_Buildbucket{
									Id:     12,
									Status: bbpb.Status_SUCCESS,
								},
							},
						},
					},
				}}
				So(saveAndGet(r).Tryjobs, ShouldResembleProto, []*apiv0pb.Tryjob{
					{
						Status: apiv0pb.Tryjob_CANCELLED,
						Result: &apiv0pb.Tryjob_Result{
							Status: apiv0pb.Tryjob_Result_FAILED_TRANSIENTLY,
							Backend: &apiv0pb.Tryjob_Result_Buildbucket_{
								Buildbucket: &apiv0pb.Tryjob_Result_Buildbucket{Id: 11},
							},
						},
					},
					{
						Status: apiv0pb.Tryjob_ENDED,
						Result: &apiv0pb.Tryjob_Result{
							Status: apiv0pb.Tryjob_Result_SUCCEEDED,
							Backend: &apiv0pb.Tryjob_Result_Buildbucket_{
								Buildbucket: &apiv0pb.Tryjob_Result_Buildbucket{Id: 12},
							},
						},
					},
				})
			})

			Convey("w/ submission", func() {
				r.Submission = &run.Submission{
					SubmittedCls: []int64{int64(cl1.ID)},
					FailedCls:    []int64{int64(cl3.ID)},
				}
				So(saveAndGet(r).Submission, ShouldResembleProto, &apiv0pb.Run_Submission{
					SubmittedClIndexes: []int32{0},
					FailedClIndexes:    []int32{2},
				})
			})
		})
	})
}
