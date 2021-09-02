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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListBuilders(t *testing.T) {
	t.Parallel()

	Convey("ListBuilders", t, func() {
		srv := &Builders{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey(`Request validation`, func() {
			Convey(`No project`, func() {
				_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{})
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "project must match")
			})

			Convey(`Invalid bucket`, func() {
				_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project: "project",
					Bucket:  "!",
				})
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "bucket must match")
			})

			Convey(`No bucket`, func() {
				_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project: "p",
				})
				So(err, ShouldHaveAppStatus, codes.Unimplemented, "request without bucket is not supported yet")
			})
		})

		Convey(`No permissions`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user",
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder1",
					Config: pb.Builder{Name: "builder"},
				},
			), ShouldBeNil)

			_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
				Project: "project",
				Bucket:  "bucket",
			})
			So(err, ShouldHaveAppStatus, codes.NotFound, "not found")
		})

		Convey(`End to end`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user",
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user",
								Role:     pb.Acl_READER,
							},
						},
					},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder1",
					Config: pb.Builder{Name: "builder1"},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder2",
					Config: pb.Builder{Name: "builder2"},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder3",
					Config: pb.Builder{Name: "builder3"},
				},
			), ShouldBeNil)

			res, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
				Project:  "project",
				Bucket:   "bucket",
				PageSize: 2,
			})
			So(err, ShouldBeNil)
			So(res.NextPageToken, ShouldNotEqual, "")
			So(res, ShouldResembleProto, &pb.ListBuildersResponse{
				NextPageToken: res.NextPageToken,
				Builders: []*pb.BuilderItem{
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Config: &pb.Builder{Name: "builder1"},
					},
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Config: &pb.Builder{Name: "builder2"},
					},
				},
			})

			res, err = srv.ListBuilders(ctx, &pb.ListBuildersRequest{
				Project:   "project",
				Bucket:    "bucket",
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ListBuildersResponse{
				Builders: []*pb.BuilderItem{
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder3",
						},
						Config: &pb.Builder{Name: "builder3"},
					},
				},
			})
		})
	})
}
