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

func TestGetBuilder(t *testing.T) {
	t.Parallel()

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
					ID:     "builder",
					Config: &pb.BuilderConfig{Name: "builder"},
				},
			), ShouldBeNil)

			_, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{Id: bid})
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
					Proto: &pb.Bucket{
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
	})
}
