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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestValidateGetBuildStatusRequest(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateGetBuildStatusRequest", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			err := validateGetBuildStatusRequest(nil)
			assert.Loosely(t, err, should.ErrLike("either id or (builder + build_number) is required"))
		})

		t.Run("empty", func(t *ftt.Test) {
			req := &pb.GetBuildStatusRequest{}
			err := validateGetBuildStatusRequest(req)
			assert.Loosely(t, err, should.ErrLike("either id or (builder + build_number) is required"))
		})

		t.Run("builder", func(t *ftt.Test) {
			req := &pb.GetBuildStatusRequest{
				Builder: &pb.BuilderID{},
			}
			err := validateGetBuildStatusRequest(req)
			assert.Loosely(t, err, should.ErrLike("either id or (builder + build_number) is required"))
		})

		t.Run("build number", func(t *ftt.Test) {
			req := &pb.GetBuildStatusRequest{
				BuildNumber: 1,
			}
			err := validateGetBuildStatusRequest(req)
			assert.Loosely(t, err, should.ErrLike("either id or (builder + build_number) is required"))
		})

		t.Run("mutual exclusion", func(t *ftt.Test) {
			t.Run("builder", func(t *ftt.Test) {
				req := &pb.GetBuildStatusRequest{
					Id:      1,
					Builder: &pb.BuilderID{},
				}
				err := validateGetBuildStatusRequest(req)
				assert.Loosely(t, err, should.ErrLike("id is mutually exclusive with (builder + build_number)"))
			})

			t.Run("build number", func(t *ftt.Test) {
				req := &pb.GetBuildStatusRequest{
					Id:          1,
					BuildNumber: 1,
				}
				err := validateGetBuildStatusRequest(req)
				assert.Loosely(t, err, should.ErrLike("id is mutually exclusive with (builder + build_number)"))
			})
		})

		t.Run("builder ID", func(t *ftt.Test) {
			t.Run("project", func(t *ftt.Test) {
				req := &pb.GetBuildStatusRequest{
					Builder:     &pb.BuilderID{},
					BuildNumber: 1,
				}
				err := validateGetBuildStatusRequest(req)
				assert.Loosely(t, err, should.ErrLike("project must match"))
			})

			t.Run("bucket", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := &pb.GetBuildStatusRequest{
						Builder: &pb.BuilderID{
							Project: "project",
						},
						BuildNumber: 1,
					}
					err := validateGetBuildStatusRequest(req)
					assert.Loosely(t, err, should.ErrLike("bucket is required"))
				})

				t.Run("v1", func(t *ftt.Test) {
					req := &pb.GetBuildStatusRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "luci.project.bucket",
							Builder: "builder",
						},
						BuildNumber: 1,
					}
					err := validateGetBuildStatusRequest(req)
					assert.Loosely(t, err, should.ErrLike("invalid use of v1 bucket in v2 API"))
				})
			})

			t.Run("builder", func(t *ftt.Test) {
				req := &pb.GetBuildStatusRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
					},
					BuildNumber: 1,
				}
				err := validateGetBuildStatusRequest(req)
				assert.Loosely(t, err, should.ErrLike("builder is required"))
			})
		})
	})
}

func TestGetBuildStatus(t *testing.T) {
	ftt.Run("GetBuildStatus", t, func(t *ftt.Test) {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		testutil.PutBucket(ctx, "project", "bucket", nil)

		t.Run("no access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:unauthorized@example.com"),
			})
			bID := 123
			bs := &model.BuildStatus{
				Build:        datastore.MakeKey(ctx, "Build", bID),
				BuildAddress: "project/bucket/builder/123",
				Status:       pb.Status_SCHEDULED,
			}
			assert.Loosely(t, datastore.Put(ctx, bs), should.BeNil)
			_, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
				Id: int64(bID),
			})
			assert.Loosely(t, err, should.ErrLike(`requested resource not found or "user:unauthorized@example.com" does not have permission to view it`))
		})

		t.Run("get build status", func(t *ftt.Test) {
			const userID = identity.Identity("user:user@example.com")
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
				),
			})

			t.Run("builder not found for BuildStatus entity", func(t *ftt.Test) {
				bID := 123
				bs := &model.BuildStatus{
					Build:  datastore.MakeKey(ctx, "Build", bID),
					Status: pb.Status_SCHEDULED,
				}
				assert.Loosely(t, datastore.Put(ctx, bs), should.BeNil)
				_, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Id: int64(bID),
				})
				assert.Loosely(t, err, should.ErrLike(`failed to parse build_address of build 123`))
			})

			t.Run("id", func(t *ftt.Test) {
				bID := 123
				bs := &model.BuildStatus{
					Build:        datastore.MakeKey(ctx, "Build", bID),
					BuildAddress: "project/bucket/builder/123",
					Status:       pb.Status_SCHEDULED,
				}
				assert.Loosely(t, datastore.Put(ctx, bs), should.BeNil)
				b, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Id: int64(bID),
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b, should.Match(&pb.Build{
					Id:     int64(bID),
					Status: pb.Status_SCHEDULED,
				}))
			})

			t.Run("id by getBuild", func(t *ftt.Test) {
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
				assert.Loosely(t, datastore.Put(ctx, bld), should.BeNil)
				b, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Id: int64(bID),
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b, should.Match(&pb.Build{
					Id:     int64(bID),
					Status: pb.Status_STARTED,
				}))
			})

			t.Run("builder + number", func(t *ftt.Test) {
				bs := &model.BuildStatus{
					Build:        datastore.MakeKey(ctx, "Build", 333),
					BuildAddress: "project/bucket/builder/3",
					Status:       pb.Status_SCHEDULED,
				}
				assert.Loosely(t, datastore.Put(ctx, bs), should.BeNil)
				b, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					BuildNumber: 3,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b, should.Match(&pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Number: 3,
					Status: pb.Status_SCHEDULED,
				}))
			})

			t.Run("builder + number by getBuild", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(ctx, &model.TagIndex{
					ID: ":2:build_address:luci.project.bucket/builder/3",
					Entries: []model.TagIndexEntry{
						{
							BuildID:  333,
							BucketID: "project/bucket",
						},
					},
				}), should.BeNil)
				b, err := srv.GetBuildStatus(ctx, &pb.GetBuildStatusRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					BuildNumber: 3,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b, should.Match(&pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Number: 3,
					Status: pb.Status_STARTED,
				}))
			})
		})
	})
}
