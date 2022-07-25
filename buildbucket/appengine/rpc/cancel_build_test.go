// Copyright 2020 The LUCI Authors.
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
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCancelBuild(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	Convey("validateCancel", t, func() {
		Convey("request", func() {
			Convey("nil", func() {
				err := validateCancel(nil)
				So(err, ShouldErrLike, "id is required")
			})

			Convey("empty", func() {
				req := &pb.CancelBuildRequest{}
				err := validateCancel(req)
				So(err, ShouldErrLike, "id is required")
			})

			Convey("id", func() {
				req := &pb.CancelBuildRequest{
					Id: 1,
				}
				err := validateCancel(req)
				So(err, ShouldErrLike, "summary_markdown is required")
			})
		})
	})

	Convey("CancelBuild", t, func() {
		srv := &Builds{}
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)

		Convey("id", func() {
			Convey("not found", func() {
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
				}
				rsp, err := srv.CancelBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
				So(sch.Tasks(), ShouldBeEmpty)
			})

			Convey("permission denied", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						// Read only permission: not enough to cancel.
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
					),
				})
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto:  &pb.Bucket{},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), ShouldBeNil)
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
				}
				rsp, err := srv.CancelBuild(ctx, req)
				So(err, ShouldErrLike, "does not have permission")
				So(rsp, ShouldBeNil)
				So(sch.Tasks(), ShouldBeEmpty)
			})

			Convey("found", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsCancel),
					),
				})
				now := testclock.TestRecentTimeLocal
				ctx, _ = testclock.UseTime(ctx, now)
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto:  &pb.Bucket{},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_STARTED,
					},
				}), ShouldBeNil)
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
					Mask: &pb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{
								"id",
								"builder",
								"update_time",
								"cancel_time",
								"status",
								"summary_markdown",
							},
						},
					},
				}
				rsp, err := srv.CancelBuild(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleProto, &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					UpdateTime:      timestamppb.New(now),
					CancelTime:      timestamppb.New(now),
					Status:          pb.Status_STARTED,
					SummaryMarkdown: "summary",
				})
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})
	})
}
