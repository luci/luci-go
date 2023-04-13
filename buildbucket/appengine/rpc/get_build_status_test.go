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
	"context"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateGetBuildStatusRequest(t *testing.T) {
	t.Parallel()
	Convey("ValidateGetBuildStatusRequest", t, func() {
		Convey("nil", func() {
			err := validateGetBuildStatusRequest(nil)
			So(err, ShouldErrLike, "either id or (builder + build_number) is required")
		})

		Convey("empty", func() {
			req := &pb.GetBuildStatusRequest{}
			err := validateGetBuildStatusRequest(req)
			So(err, ShouldErrLike, "either id or (builder + build_number) is required")
		})

		Convey("builder", func() {
			req := &pb.GetBuildStatusRequest{
				Builder: &pb.BuilderID{},
			}
			err := validateGetBuildStatusRequest(req)
			So(err, ShouldErrLike, "either id or (builder + build_number) is required")
		})

		Convey("build number", func() {
			req := &pb.GetBuildStatusRequest{
				BuildNumber: 1,
			}
			err := validateGetBuildStatusRequest(req)
			So(err, ShouldErrLike, "either id or (builder + build_number) is required")
		})

		Convey("mutual exclusion", func() {
			Convey("builder", func() {
				req := &pb.GetBuildStatusRequest{
					Id:      1,
					Builder: &pb.BuilderID{},
				}
				err := validateGetBuildStatusRequest(req)
				So(err, ShouldErrLike, "id is mutually exclusive with (builder + build_number)")
			})

			Convey("build number", func() {
				req := &pb.GetBuildStatusRequest{
					Id:          1,
					BuildNumber: 1,
				}
				err := validateGetBuildStatusRequest(req)
				So(err, ShouldErrLike, "id is mutually exclusive with (builder + build_number)")
			})
		})

		Convey("builder ID", func() {
			Convey("project", func() {
				req := &pb.GetBuildStatusRequest{
					Builder:     &pb.BuilderID{},
					BuildNumber: 1,
				}
				err := validateGetBuildStatusRequest(req)
				So(err, ShouldErrLike, "project must match")
			})

			Convey("bucket", func() {
				Convey("empty", func() {
					req := &pb.GetBuildStatusRequest{
						Builder: &pb.BuilderID{
							Project: "project",
						},
						BuildNumber: 1,
					}
					err := validateGetBuildStatusRequest(req)
					So(err, ShouldErrLike, "bucket is required")
				})

				Convey("v1", func() {
					req := &pb.GetBuildStatusRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "luci.project.bucket",
							Builder: "builder",
						},
						BuildNumber: 1,
					}
					err := validateGetBuildStatusRequest(req)
					So(err, ShouldErrLike, "invalid use of v1 bucket in v2 API")
				})
			})

			Convey("builder", func() {
				req := &pb.GetBuildStatusRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
					},
					BuildNumber: 1,
				}
				err := validateGetBuildStatusRequest(req)
				So(err, ShouldErrLike, "builder is required")
			})
		})
	})
}

func TestGetBuildStatus(t *testing.T) {
	Convey("GetBuildStatus", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		testutil.PutBucket(ctx, "project", "bucket", nil)

		Convey("no access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:unauthorized@example.com"),
			})
			bID := 123
			bs := &model.BuildStatus{
				Build:        datastore.MakeKey(ctx, "Build", bID),
				BuildAddress: "project/bucket/builder/123",
				Status:       pb.Status_SCHEDULED,
			}
			So(datastore.Put(ctx, bs), ShouldBeNil)
			_, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
				Id: int64(bID),
			})
			So(err, ShouldErrLike, `requested resource not found or "user:unauthorized@example.com" does not have permission to view it`)
		})

		Convey("get build status", func() {
			const userID = identity.Identity("user:user@example.com")
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
				),
			})

			Convey("builder not found for BuildStatus entity", func() {
				bID := 123
				bs := &model.BuildStatus{
					Build:  datastore.MakeKey(ctx, "Build", bID),
					Status: pb.Status_SCHEDULED,
				}
				So(datastore.Put(ctx, bs), ShouldBeNil)
				_, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Id: int64(bID),
				})
				So(err, ShouldErrLike, `failed to parse build_address of build 123`)
			})

			Convey("id", func() {
				bID := 123
				bs := &model.BuildStatus{
					Build:        datastore.MakeKey(ctx, "Build", bID),
					BuildAddress: "project/bucket/builder/123",
					Status:       pb.Status_SCHEDULED,
				}
				So(datastore.Put(ctx, bs), ShouldBeNil)
				b, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Id: int64(bID),
				})
				So(err, ShouldBeNil)
				So(b, ShouldResembleProto, &pb.Build{
					Id:     int64(bID),
					Status: pb.Status_SCHEDULED,
				})
			})

			Convey("id by getBuild", func() {
				bID := 123
				bld := &model.Build{
					ID: 123,
					Proto: &pb.Build{
						Id: 123,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_STARTED,
					},
				}
				So(datastore.Put(ctx, bld), ShouldBeNil)
				b, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Id: int64(bID),
				})
				So(err, ShouldBeNil)
				So(b, ShouldResembleProto, &pb.Build{
					Id:     int64(bID),
					Status: pb.Status_STARTED,
				})
			})

			Convey("builder + number", func() {
				bs := &model.BuildStatus{
					Build:        datastore.MakeKey(ctx, "Build", 333),
					BuildAddress: "project/bucket/builder/3",
					Status:       pb.Status_SCHEDULED,
				}
				So(datastore.Put(ctx, bs), ShouldBeNil)
				b, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					BuildNumber: 3,
				})
				So(err, ShouldBeNil)
				So(b, ShouldResembleProto, &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Number: 3,
					Status: pb.Status_SCHEDULED,
				})
			})

			Convey("builder + number by getBuild", func() {
				So(datastore.Put(ctx, &model.Build{
					ID: 333,
					Proto: &pb.Build{
						Id: 333,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Number: 3,
						Status: pb.Status_STARTED,
					},
					BucketID: "project/bucket",
					Tags:     []string{"build_address:luci.project.bucket/builder/1"},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.TagIndex{
					ID: ":2:build_address:luci.project.bucket/builder/3",
					Entries: []model.TagIndexEntry{
						{
							BuildID:  333,
							BucketID: "project/bucket",
						},
					},
				}), ShouldBeNil)
				b, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					BuildNumber: 3,
				})
				So(err, ShouldBeNil)
				So(b, ShouldResembleProto, &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Number: 3,
					Status: pb.Status_STARTED,
				})
			})
		})
	})
}
