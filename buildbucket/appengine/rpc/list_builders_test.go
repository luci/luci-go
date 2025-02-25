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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestListBuilders(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	ftt.Run("ListBuilders", t, func(t *ftt.Test) {
		srv := &Builders{}
		ctx := memory.Use(context.Background())
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t.Run(`Request validation`, func(t *ftt.Test) {
			t.Run(`No project when bucket is specified`, func(t *ftt.Test) {
				_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{Bucket: "bucket"})
				assert.Loosely(t, appstatus.Code(err), should.Match(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("project must be specified"))
			})

			t.Run(`Invalid bucket`, func(t *ftt.Test) {
				_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project: "project",
					Bucket:  "!",
				})
				assert.Loosely(t, appstatus.Code(err), should.Match(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bucket must match"))
			})

			t.Run(`Invalid page token`, func(t *ftt.Test) {
				_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:   "project",
					Bucket:    "bucket",
					PageToken: "invalid token",
				})
				assert.Loosely(t, appstatus.Code(err), should.Match(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("invalid page token"))
			})
		})

		t.Run(`No permissions`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
			})
			assert.Loosely(t, datastore.Put(
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
			), should.BeNil)

			_, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
				Project: "project",
				Bucket:  "bucket",
			})
			assert.Loosely(t, appstatus.Code(err), should.Match(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("not found"))
		})

		t.Run(`End to end`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket1", bbperms.BuildersList),
					authtest.MockPermission(userID, "project:bucket3", bbperms.BuildersList),
					authtest.MockPermission(userID, "project2:bucket1", bbperms.BuildersList),
				),
			})
			assert.Loosely(t, datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket1",
					Proto:  &pb.Bucket{},
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
					Proto:  &pb.Bucket{},
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
					Proto:  &pb.Bucket{},
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
			), should.BeNil)

			t.Run(`List all builders in bucket`, func(t *ftt.Test) {
				res, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:  "project",
					Bucket:   "bucket1",
					PageSize: 2,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.NotEqual(""))
				assert.Loosely(t, res.GetBuilders(), should.Match([]*pb.BuilderItem{
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
				}))

				res, err = srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:   "project",
					Bucket:    "bucket1",
					PageToken: res.NextPageToken,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
				assert.Loosely(t, res.GetBuilders(), should.Match([]*pb.BuilderItem{
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder3",
						},
						Config: &pb.BuilderConfig{Name: "builder3"},
					},
				}))
			})

			t.Run(`List all builders in project`, func(t *ftt.Test) {
				res, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:  "project",
					PageSize: 2,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.NotEqual(""))
				assert.Loosely(t, res.GetBuilders(), should.Match([]*pb.BuilderItem{
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
				}))

				res, err = srv.ListBuilders(ctx, &pb.ListBuildersRequest{
					Project:   "project",
					PageToken: res.NextPageToken,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
				assert.Loosely(t, res.GetBuilders(), should.Match([]*pb.BuilderItem{
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
				}))
			})

			t.Run(`List all builders`, func(t *ftt.Test) {
				res, err := srv.ListBuilders(ctx, &pb.ListBuildersRequest{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
				assert.Loosely(t, res.GetBuilders(), should.Match([]*pb.BuilderItem{
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
				}))
			})
		})
	})
}
