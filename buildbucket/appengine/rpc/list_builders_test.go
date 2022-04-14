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

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"

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
		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		So(err, ShouldBeNil)
		aead, err := aead.New(kh)
		So(err, ShouldBeNil)
		ctx = secrets.SetPrimaryTinkAEADForTest(ctx, aead)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey(`Request validation`, func() {
			Convey(`No project when bucket is specified`, func() {
				_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{Bucket: "bucket"})
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "project must be specified")
			})

			Convey(`Invalid bucket`, func() {
				_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project: "project",
					Bucket:  "!",
				})
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "bucket must match")
			})

			Convey(`Invalid page token`, func() {
				_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:   "project",
					Bucket:    "bucket",
					PageToken: "invalid token",
				})
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page token")
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
					Config: &pb.BuilderConfig{Name: "builder"},
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
					ID:     "bucket1",
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
					Parent: model.BucketKey(ctx, "project", "bucket1"),
					ID:     "builder1",
					Config: &pb.BuilderConfig{Name: "builder1"},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket1"),
					ID:     "builder2",
					Config: &pb.BuilderConfig{Name: "builder2"},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket1"),
					ID:     "builder3",
					Config: &pb.BuilderConfig{Name: "builder3"},
				},

				// Bucket without permission
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket2",
					Proto:  &pb.Bucket{},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket2"),
					ID:     "builder1",
					Config: &pb.BuilderConfig{Name: "builder1"},
				},

				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket3",
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
					Parent: model.BucketKey(ctx, "project", "bucket3"),
					ID:     "builder1",
					Config: &pb.BuilderConfig{Name: "builder1"},
				},

				// Bucket in another project.
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project2"),
					ID:     "bucket1",
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
					Parent: model.BucketKey(ctx, "project2", "bucket1"),
					ID:     "builder1",
					Config: &pb.BuilderConfig{Name: "builder1"},
				},

				// Bucket in another project without permission.
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project2"),
					ID:     "bucket2",
					Proto:  &pb.Bucket{},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project2", "bucket2"),
					ID:     "builder1",
					Config: &pb.BuilderConfig{Name: "builder1"},
				},
			), ShouldBeNil)

			Convey(`List all builders in bucket`, func() {
				res, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:  "project",
					Bucket:   "bucket1",
					PageSize: 2,
				})
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldNotEqual, "")
				So(res.GetBuilders(), ShouldResembleProto, []*pb.BuilderItem{
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder1",
						},
						Config: &pb.BuilderConfig{Name: "builder1"},
					},
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder2",
						},
						Config: &pb.BuilderConfig{Name: "builder2"},
					},
				})

				res, err = srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:   "project",
					Bucket:    "bucket1",
					PageToken: res.NextPageToken,
				})
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldBeEmpty)
				So(res.GetBuilders(), ShouldResembleProto, []*pb.BuilderItem{
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder3",
						},
						Config: &pb.BuilderConfig{Name: "builder3"},
					},
				})
			})

			Convey(`List all builders in project`, func() {
				res, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:  "project",
					PageSize: 2,
				})
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldNotEqual, "")
				So(res.GetBuilders(), ShouldResembleProto, []*pb.BuilderItem{
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder1",
						},
						Config: &pb.BuilderConfig{Name: "builder1"},
					},
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder2",
						},
						Config: &pb.BuilderConfig{Name: "builder2"},
					},
				})

				res, err = srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:   "project",
					PageToken: res.NextPageToken,
				})
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldBeEmpty)
				So(res.GetBuilders(), ShouldResembleProto, []*pb.BuilderItem{
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder3",
						},
						Config: &pb.BuilderConfig{Name: "builder3"},
					},
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket3",
							Builder: "builder1",
						},
						Config: &pb.BuilderConfig{Name: "builder1"},
					},
				})
			})

			Convey(`List all builders`, func() {
				res, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{})
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldEqual, "")
				So(res.GetBuilders(), ShouldResembleProto, []*pb.BuilderItem{
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder1",
						},
						Config: &pb.BuilderConfig{Name: "builder1"},
					},
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder2",
						},
						Config: &pb.BuilderConfig{Name: "builder2"},
					},
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder3",
						},
						Config: &pb.BuilderConfig{Name: "builder3"},
					},
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket3",
							Builder: "builder1",
						},
						Config: &pb.BuilderConfig{Name: "builder1"},
					},
					{
						Id: &pb.BuilderID{
							Project: "project2",
							Bucket:  "bucket1",
							Builder: "builder1",
						},
						Config: &pb.BuilderConfig{Name: "builder1"},
					},
				})
			})
		})
	})
}
