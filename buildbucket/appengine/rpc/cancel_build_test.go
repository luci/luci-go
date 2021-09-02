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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCancelBuild(t *testing.T) {
	t.Parallel()

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
					Identity: "user:user",
				})
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user",
								Role:     pb.Acl_READER,
							},
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.Build{
					Proto: pb.Build{
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
					Identity: "user:user",
				})
				now := testclock.TestRecentTimeLocal
				ctx, _ = testclock.UseTime(ctx, now)
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user",
								Role:     pb.Acl_SCHEDULER,
							},
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.Build{
					Proto: pb.Build{
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
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleProto, &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					EndTime: timestamppb.New(now),
					Input:   &pb.Build_Input{},
					Status:  pb.Status_CANCELED,
				})
				So(sch.Tasks(), ShouldHaveLength, 2)
			})

			Convey("ended", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:user",
				})
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user",
								Role:     pb.Acl_SCHEDULER,
							},
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.Build{
					Proto: pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_SUCCESS,
					},
				}), ShouldBeNil)
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
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
					Input:  &pb.Build_Input{},
					Status: pb.Status_SUCCESS,
				})
				So(sch.Tasks(), ShouldBeEmpty)
			})

			Convey("task cancellation", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:user",
				})
				now := testclock.TestRecentTimeLocal
				ctx, _ = testclock.UseTime(ctx, now)
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user",
								Role:     pb.Acl_SCHEDULER,
							},
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.Build{
					Proto: pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.BuildInfra{
					Build: datastore.MakeKey(ctx, "Build", 1),
					Proto: model.DSBuildInfra{
						BuildInfra: pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{
								Hostname: "example.com",
								TaskId:   "id",
							},
						},
					},
				}), ShouldBeNil)
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
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
					EndTime: timestamppb.New(now),
					Input:   &pb.Build_Input{},
					Status:  pb.Status_CANCELED,
				})
				So(sch.Tasks(), ShouldHaveLength, 3)
			})

			Convey("resultdb finalization", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:user",
				})
				now := testclock.TestRecentTimeLocal
				ctx, _ = testclock.UseTime(ctx, now)
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user",
								Role:     pb.Acl_SCHEDULER,
							},
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.Build{
					Proto: pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.BuildInfra{
					Build: datastore.MakeKey(ctx, "Build", 1),
					Proto: model.DSBuildInfra{
						BuildInfra: pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:   "example.com",
								Invocation: "id",
							},
						},
					},
				}), ShouldBeNil)
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
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
					EndTime: timestamppb.New(now),
					Input:   &pb.Build_Input{},
					Status:  pb.Status_CANCELED,
				})
				So(sch.Tasks(), ShouldHaveLength, 3)
			})
		})
	})
}
