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

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestGetBuilder(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	ftt.Run("GetBuilder", t, func(t *ftt.Test) {
		srv := &Builders{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		bid := &pb.BuilderID{
			Project: "project",
			Bucket:  "bucket",
			Builder: "builder",
		}

		t.Run(`Request validation`, func(t *ftt.Test) {
			t.Run(`Invalid ID`, func(t *ftt.Test) {
				_, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{})
				assert.Loosely(t, appstatus.Code(err), should.Match(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("id: project must match"))
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
					ID:     "builder",
					Config: &pb.BuilderConfig{Name: "builder"},
				},
			), should.BeNil)

			_, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{Id: bid})
			assert.Loosely(t, appstatus.Code(err), should.Match(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("not found"))
		})

		t.Run(`End to end`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
				),
			})
			assert.Loosely(t, datastore.Put(
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
			), should.BeNil)

			res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{Id: bid})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.BuilderItem{
				Id:     bid,
				Config: &pb.BuilderConfig{Name: "builder"},
			}))
		})

		t.Run(`metadata`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
				),
			})
			assert.Loosely(t, datastore.Put(
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
			), should.BeNil)
			res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{
				Id: bid,
				Mask: &pb.BuilderMask{
					Type: pb.BuilderMask_ALL,
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.BuilderItem{
				Id:     bid,
				Config: &pb.BuilderConfig{Name: "builder"},
				Metadata: &pb.BuilderMetadata{
					Owner: "owner",
					Health: &pb.HealthStatus{
						HealthScore: 9,
					},
				},
			}))
			bid.Builder = "builder2"
			res, err = srv.GetBuilder(ctx, &pb.GetBuilderRequest{
				Id: bid,
				Mask: &pb.BuilderMask{
					Type: pb.BuilderMask_ALL,
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.BuilderItem{
				Id:     bid,
				Config: &pb.BuilderConfig{Name: "builder2"},
			}))
		})

		t.Run(`mask`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
				),
			})
			assert.Loosely(t, datastore.Put(
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
			), should.BeNil)

			t.Run(`default mask`, func(t *ftt.Test) {
				res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{Id: bid})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.BuilderItem{
					Id:     bid,
					Config: &pb.BuilderConfig{Name: "builder"},
				}))
			})

			t.Run(`config only`, func(t *ftt.Test) {
				res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{
					Id: bid,
					Mask: &pb.BuilderMask{
						Type: pb.BuilderMask_CONFIG_ONLY,
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.BuilderItem{
					Id:     bid,
					Config: &pb.BuilderConfig{Name: "builder"},
				}))
			})

			t.Run(`metadata only`, func(t *ftt.Test) {
				res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{
					Id: bid,
					Mask: &pb.BuilderMask{
						Type: pb.BuilderMask_METADATA_ONLY,
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.BuilderItem{
					Id: bid,
					Metadata: &pb.BuilderMetadata{
						Owner: "owner",
						Health: &pb.HealthStatus{
							HealthScore: 9,
						},
					},
				}))
			})

			t.Run(`all`, func(t *ftt.Test) {
				res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{
					Id: bid,
					Mask: &pb.BuilderMask{
						Type: pb.BuilderMask_ALL,
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.BuilderItem{
					Id:     bid,
					Config: &pb.BuilderConfig{Name: "builder"},
					Metadata: &pb.BuilderMetadata{
						Owner: "owner",
						Health: &pb.HealthStatus{
							HealthScore: 9,
						},
					},
				}))
			})
		})

		t.Run(`shadow`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket.shadow", bbperms.BuildersGet),
				),
			})
			assert.Loosely(t, datastore.Put(
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
						Dimensions:     []string{"os:Linux", "pool:luci.chromium.try", "todelete:random"},
						Properties:     `{"a":"b"}`,
						ShadowBuilderAdjustments: &pb.BuilderConfig_ShadowBuilderAdjustments{
							ServiceAccount: "shadow@example.com",
							Pool:           "shadow.pool",
							Properties:     `{"a":"b2"}`,
							Dimensions: []string{
								"pool:shadow.pool",
								"todelete:",
								"120:toadd:v",
							},
						},
					},
				},
			), should.BeNil)

			shadowBID := &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket.shadow",
				Builder: "builder",
			}
			res, err := srv.GetBuilder(ctx, &pb.GetBuilderRequest{Id: shadowBID})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.BuilderItem{
				Id: shadowBID,
				Config: &pb.BuilderConfig{
					Name:           "builder",
					ServiceAccount: "shadow@example.com",
					Dimensions:     []string{"120:toadd:v", "os:Linux", "pool:shadow.pool"},
					Properties:     `{"a":"b2"}`,
				},
			}))
		})
	})
}
