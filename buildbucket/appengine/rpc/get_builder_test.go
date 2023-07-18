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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetBuilder(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	Convey("GetBuilder", t, func() {
		srv := &Builders{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		bid := &pb.BuilderID{
			Project: "project",
			Bucket:  "bucket",
			Builder: "builder",
		}

		Convey(`Request validation`, func() {
			Convey(`Invalid ID`, func() {
				_, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{})
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "id: project must match")
			})
		})

		Convey(`No permissions`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder",
					Config: &pb.BuilderConfig{Name: "builder"},
				},
			), ShouldBeNil)

			_, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{Id: bid})
			So(err, ShouldHaveAppStatus, codes.NotFound, "not found")
		})

		Convey(`End to end`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
				),
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
					Proto:  &pb.Bucket{},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder",
					Config: &pb.BuilderConfig{Name: "builder"},
				},
			), ShouldBeNil)

			res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{Id: bid})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.BuilderItem{
				Id:     bid,
				Config: &pb.BuilderConfig{Name: "builder"},
			})
		})

		Convey(`metadata`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
				),
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
					Proto:  &pb.Bucket{},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder",
					Config: &pb.BuilderConfig{Name: "builder"},
					Metadata: &pb.BuilderMetadata{
						Owner: "owner",
						Health: &pb.HealthStatus{
							HealthScore: 9,
						},
					},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder2",
					Config: &pb.BuilderConfig{Name: "builder2"},
				},
			), ShouldBeNil)
			res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{
				Id: bid,
				Mask: &pb.BuilderMask{
					Type: pb.BuilderMask_ALL,
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.BuilderItem{
				Id:     bid,
				Config: &pb.BuilderConfig{Name: "builder"},
				Metadata: &pb.BuilderMetadata{
					Owner: "owner",
					Health: &pb.HealthStatus{
						HealthScore: 9,
					},
				},
			})
			bid.Builder = "builder2"
			res, err = srv.GetBuilder(ctx, &pb.GetBuilderRequest{
				Id: bid,
				Mask: &pb.BuilderMask{
					Type: pb.BuilderMask_ALL,
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.BuilderItem{
				Id:     bid,
				Config: &pb.BuilderConfig{Name: "builder2"},
			})
		})

		Convey(`mask`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
				),
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
					Proto:  &pb.Bucket{},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder",
					Config: &pb.BuilderConfig{Name: "builder"},
					Metadata: &pb.BuilderMetadata{
						Owner: "owner",
						Health: &pb.HealthStatus{
							HealthScore: 9,
						},
					},
				},
			), ShouldBeNil)

			Convey(`default mask`, func() {
				res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{Id: bid})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.BuilderItem{
					Id:     bid,
					Config: &pb.BuilderConfig{Name: "builder"},
				})
			})

			Convey(`config only`, func() {
				res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{
					Id: bid,
					Mask: &pb.BuilderMask{
						Type: pb.BuilderMask_CONFIG_ONLY,
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.BuilderItem{
					Id:     bid,
					Config: &pb.BuilderConfig{Name: "builder"},
				})
			})

			Convey(`metadata only`, func() {
				res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{
					Id: bid,
					Mask: &pb.BuilderMask{
						Type: pb.BuilderMask_METADATA_ONLY,
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.BuilderItem{
					Id: bid,
					Metadata: &pb.BuilderMetadata{
						Owner: "owner",
						Health: &pb.HealthStatus{
							HealthScore: 9,
						},
					},
				})
			})

			Convey(`all`, func() {
				res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{
					Id: bid,
					Mask: &pb.BuilderMask{
						Type: pb.BuilderMask_ALL,
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.BuilderItem{
					Id:     bid,
					Config: &pb.BuilderConfig{Name: "builder"},
					Metadata: &pb.BuilderMetadata{
						Owner: "owner",
						Health: &pb.HealthStatus{
							HealthScore: 9,
						},
					},
				})
			})
		})

		Convey(`shadow`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket.shadow", bbperms.BuildersGet),
				),
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
					Proto: &pb.Bucket{
						Shadow: "bucket.shadow",
					},
				},
				&model.Bucket{
					Parent:  model.ProjectKey(ctx, "project"),
					ID:      "bucket.shadow",
					Proto:   &pb.Bucket{},
					Shadows: []string{"bucket"},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder",
					Config: &pb.BuilderConfig{
						Name:           "builder",
						ServiceAccount: "example@example.com",
						Dimensions:     []string{"os:Linux", "pool:luci.chromium.try"},
						Properties:     `{"a":"b"}`,
						ShadowBuilderAdjustments: &pb.BuilderConfig_ShadowBuilderAdjustments{
							ServiceAccount: "shadow@example.com",
							Pool:           "shadow.pool",
							Properties:     `{"a":"b2"}`,
						},
					},
				},
			), ShouldBeNil)

			shadowBID := &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket.shadow",
				Builder: "builder",
			}
			res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{Id: shadowBID})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.BuilderItem{
				Id: shadowBID,
				Config: &pb.BuilderConfig{
					Name:           "builder",
					ServiceAccount: "shadow@example.com",
					Dimensions:     []string{"os:Linux", "pool:shadow.pool"},
					Properties:     `{"a":"b2"}`,
				},
			})
		})
	})
}
