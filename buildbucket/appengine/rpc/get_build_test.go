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

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetBuild(t *testing.T) {
	t.Parallel()

	Convey("GetBuild", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("validation", func() {
			Convey("request", func() {
				Convey("nil", func() {
					rsp, err := srv.GetBuild(ctx, nil)
					So(err, ShouldErrLike, "id or (builder and build_number) is required")
					So(rsp, ShouldBeNil)
				})

				Convey("empty", func() {
					req := &pb.GetBuildRequest{}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "id or (builder and build_number) is required")
					So(rsp, ShouldBeNil)
				})

				Convey("builder", func() {
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{},
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "id or (builder and build_number) is required")
					So(rsp, ShouldBeNil)
				})

				Convey("build number", func() {
					req := &pb.GetBuildRequest{
						BuildNumber: 1,
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "id or (builder and build_number) is required")
					So(rsp, ShouldBeNil)
				})
			})

			Convey("mutual exclusion", func() {
				Convey("builder", func() {
					req := &pb.GetBuildRequest{
						Id:      1,
						Builder: &pb.BuilderID{},
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "id is mutually exclusive with (builder and build_number)")
					So(rsp, ShouldBeNil)
				})

				Convey("build number", func() {
					req := &pb.GetBuildRequest{
						Id:          1,
						BuildNumber: 1,
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "id is mutually exclusive with (builder and build_number)")
					So(rsp, ShouldBeNil)
				})
			})

			Convey("builder", func() {
				Convey("project", func() {
					req := &pb.GetBuildRequest{
						Builder:     &pb.BuilderID{},
						BuildNumber: 1,
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "project must match")
					So(rsp, ShouldBeNil)
				})

				Convey("bucket", func() {
					Convey("empty", func() {
						req := &pb.GetBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
							},
							BuildNumber: 1,
						}
						rsp, err := srv.GetBuild(ctx, req)
						So(err, ShouldErrLike, "bucket must match")
						So(rsp, ShouldBeNil)
					})

					Convey("v1", func() {
						req := &pb.GetBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "luci.project.bucket",
								Builder: "builder",
							},
							BuildNumber: 1,
						}
						rsp, err := srv.GetBuild(ctx, req)
						So(err, ShouldErrLike, "invalid use of v1 builder.bucket in v2 API")
						So(rsp, ShouldBeNil)
					})
				})

				Convey("builder", func() {
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
						},
						BuildNumber: 1,
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "builder must match")
					So(rsp, ShouldBeNil)
				})
			})
		})

		Convey("id", func() {
			Convey("not found", func() {
				req := &pb.GetBuildRequest{
					Id: 1,
				}
				rsp, err := srv.GetBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
			})

			Convey("found", func() {
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
				req := &pb.GetBuildRequest{
					Id: 1,
				}
				rsp, err := srv.GetBuild(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Input: &pb.Build_Input{},
				})
			})
		})
	})
}
