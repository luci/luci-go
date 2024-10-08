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

	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestValidateSetBuilderHealthRequest(t *testing.T) {
	t.Parallel()
	ftt.Run("validateSetBuilderHealthRequest", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testutil.PutBucket(ctx, "project", "bucket", nil)

		t.Run("empty req", func(t *ftt.Test) {
			req := &pb.SetBuilderHealthRequest{}
			err := validateRequest(ctx, req, nil, nil)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("ok req", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "project:bucket", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:builder1", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:builder2", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:builder3", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Bucket:  "bucket",
							Project: "project",
							Builder: "builder1",
						},
						Health: &pb.HealthStatus{HealthScore: 10},
					},
					{
						Id: &pb.BuilderID{
							Bucket:  "bucket",
							Project: "project",
							Builder: "builder2",
						},
						Health: &pb.HealthStatus{HealthScore: 0},
					},
					{
						Id: &pb.BuilderID{
							Bucket:  "bucket",
							Project: "project",
							Builder: "builder3",
						},
						Health: &pb.HealthStatus{HealthScore: 4},
					},
				},
			}
			resp := make([]*pb.SetBuilderHealthResponse_Response, 3)
			err := validateRequest(ctx, req, nil, resp)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Resemble([]*pb.SetBuilderHealthResponse_Response{
				nil, nil, nil,
			}))
		})

		t.Run("miltiple entries", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "project:bucket", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:builder", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Bucket:  "bucket",
							Project: "project",
							Builder: "builder",
						},
						Health: &pb.HealthStatus{HealthScore: 10},
					},
					{
						Id: &pb.BuilderID{
							Bucket:  "bucket",
							Project: "project",
							Builder: "builder",
						},
						Health: &pb.HealthStatus{HealthScore: 0},
					},
					{
						Id: &pb.BuilderID{
							Bucket:  "bucket",
							Project: "project",
							Builder: "builder",
						},
						Health: &pb.HealthStatus{HealthScore: 4},
					},
				},
			}
			resp := make([]*pb.SetBuilderHealthResponse_Response, 3)
			err := validateRequest(ctx, req, nil, resp)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.Equal("The following builder has multiple entries: project/bucket/builder"))
		})

		t.Run("bad health score", func(t *ftt.Test) {
			errs := map[int]error{}
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "project:bucket", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:builder", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Bucket:  "bucket",
							Project: "project",
							Builder: "builder",
						},
						Health: &pb.HealthStatus{HealthScore: 11},
					},
				},
			}
			resp := make([]*pb.SetBuilderHealthResponse_Response, 1)
			err := validateRequest(ctx, req, errs, resp)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Resemble([]*pb.SetBuilderHealthResponse_Response{
				{
					Response: &pb.SetBuilderHealthResponse_Response_Error{
						Error: &status.Status{
							Code:    3,
							Message: "Builder: project/bucket/builder: HealthScore should be between 0 and 10",
						},
					},
				},
			}))
		})

		t.Run("builderID not present, health is", func(t *ftt.Test) {
			errs := map[int]error{}
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "project:bucket", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:builder", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Health: &pb.HealthStatus{HealthScore: 11},
					},
				},
			}
			err := validateRequest(ctx, req, errs, nil)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring(".health[0].id: required"))
		})
	})
}

func TestSetBuilderHealth(t *testing.T) {
	t.Parallel()

	ftt.Run("requests", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		srv := &Builders{}
		testutil.PutBucket(ctx, "chrome", "cq", nil)
		testutil.PutBucket(ctx, "chromeos", "cq", nil)

		t.Run("bad request; no perms", func(t *ftt.Test) {
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Health: &pb.HealthStatus{HealthScore: 12},
					},
					{
						Id: &pb.BuilderID{
							Project: "project2",
							Bucket:  "bucket2",
							Builder: "builder2",
						},
						Health: &pb.HealthStatus{HealthScore: 13},
					},
				},
			}
			resp, err := srv.SetBuilderHealth(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Resemble(&pb.SetBuilderHealthResponse{
				Responses: []*pb.SetBuilderHealthResponse_Response{
					{
						Response: &pb.SetBuilderHealthResponse_Response_Error{
							Error: &status.Status{
								Code:    7,
								Message: "Builder: project/bucket/builder: attaching a status: rpc error: code = NotFound desc = requested resource not found or \"anonymous:anonymous\" does not have permission to view it",
							},
						},
					},
					{
						Response: &pb.SetBuilderHealthResponse_Response_Error{
							Error: &status.Status{
								Code:    7,
								Message: "Builder: project2/bucket2/builder2: attaching a status: rpc error: code = NotFound desc = requested resource not found or \"anonymous:anonymous\" does not have permission to view it",
							},
						},
					},
				},
			}))
		})

		t.Run("bad request; has perms", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: model.BucketKey(ctx, "chrome", "cq"),
			}), should.BeNil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chrome:cq", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:amd-cq", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq",
						},
						Health: &pb.HealthStatus{HealthScore: 12},
					},
					{
						Id:     &pb.BuilderID{},
						Health: &pb.HealthStatus{HealthScore: 13},
					},
				},
			}
			_, err := srv.SetBuilderHealth(ctx, req)
			assert.Loosely(t, err.Error(), should.ContainSubstring(".health[1].id.project: required (and 2 other errors)"))
		})

		t.Run("bad req; one no perm, one validation err, one no builder saved", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: model.BucketKey(ctx, "chrome", "cq"),
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: model.BucketKey(ctx, "chromeos", "cq"),
			}), should.BeNil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chrome:cq", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:amd-cq", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:amd-cq-2", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq",
						},
						Health: &pb.HealthStatus{HealthScore: 12},
					},
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq-2",
						},
						Health: &pb.HealthStatus{HealthScore: 8},
					},
					{
						Id: &pb.BuilderID{
							Project: "chromeos",
							Bucket:  "cq",
							Builder: "amd-cq",
						},
						Health: &pb.HealthStatus{HealthScore: 12},
					},
				},
			}
			resp, err := srv.SetBuilderHealth(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Resemble(&pb.SetBuilderHealthResponse{
				Responses: []*pb.SetBuilderHealthResponse_Response{
					{
						Response: &pb.SetBuilderHealthResponse_Response_Error{
							Error: &status.Status{
								Message: "Builder: chrome/cq/amd-cq: HealthScore should be between 0 and 10",
								Code:    3,
							},
						},
					},
					{
						Response: &pb.SetBuilderHealthResponse_Response_Error{
							Error: &status.Status{
								Message: "attaching a status: rpc error: code = Internal desc = failed to get builder amd-cq-2: datastore: no such entity",
								Code:    13,
							},
						},
					},
					{
						Response: &pb.SetBuilderHealthResponse_Response_Error{
							Error: &status.Status{
								Message: "Builder: chromeos/cq/amd-cq: attaching a status: rpc error: code = NotFound desc = requested resource not found or \"user:someone@example.com\" does not have permission to view it",
								Code:    7,
							},
						},
					},
				},
			}))
		})
	})

	ftt.Run("existing entities", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "fake-cr-buildbucket")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)
		srv := &Builders{}
		testutil.PutBucket(ctx, "chrome", "cq", nil)

		t.Run("builders exists; update is normal", func(t *ftt.Test) {
			bktKey := model.BucketKey(ctx, "chrome", "cq")

			bldrToPut1 := &model.Builder{
				ID:     "amd-cq",
				Parent: bktKey,
				Config: &pb.BuilderConfig{
					BuilderHealthMetricsLinks: &pb.BuilderConfig_BuilderHealthLinks{
						DataLinks: map[string]string{
							"user": "data-link-for-amd-cq",
						},
						DocLinks: map[string]string{
							"user": "doc-link-for-amd-cq",
						},
					},
				},
			}
			bldrToPut2 := &model.Builder{
				ID:     "amd-cq-2",
				Parent: bktKey,
				Config: &pb.BuilderConfig{
					BuilderHealthMetricsLinks: &pb.BuilderConfig_BuilderHealthLinks{
						DataLinks: map[string]string{
							"user": "data-link-for-amd-cq-2",
						},
						DocLinks: map[string]string{
							"user": "doc-link-for-amd-cq-2",
						},
					},
				},
			}
			bldrToPut3 := &model.Builder{
				ID:     "amd-cq-3",
				Parent: bktKey,
			}
			assert.Loosely(t, datastore.Put(ctx, bldrToPut1, bldrToPut2, bldrToPut3), should.BeNil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chrome:cq", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:amd-cq", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:amd-cq-2", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:amd-cq-3", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq",
						},
						Health: &pb.HealthStatus{
							HealthScore: 9,
							DataLinks: map[string]string{
								"user": "data-link-for-amd-cq-from-req",
							},
							DocLinks: map[string]string{
								"user": "doc-link-for-amd-cq-from-req",
							},
						},
					},
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq-2",
						},
						Health: &pb.HealthStatus{
							HealthScore: 8,
						},
					},
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq-3",
						},
						Health: &pb.HealthStatus{
							HealthScore: 2,
						},
					},
				},
			}
			_, err := srv.SetBuilderHealth(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			expectedBuilder1 := &model.Builder{ID: "amd-cq", Parent: bktKey}
			expectedBuilder2 := &model.Builder{ID: "amd-cq-2", Parent: bktKey}
			expectedBuilder3 := &model.Builder{ID: "amd-cq-3", Parent: bktKey}
			assert.Loosely(t, datastore.Get(ctx, expectedBuilder1, expectedBuilder2, expectedBuilder3), should.BeNil)
			assert.Loosely(t, expectedBuilder1.Metadata.Health.HealthScore, should.Equal(9))
			assert.Loosely(t, expectedBuilder1.Metadata.Health.Reporter, should.Equal("someone@example.com"))
			assert.Loosely(t, expectedBuilder1.Metadata.Health.ReportedTime, should.NotBeNil)
			assert.Loosely(t, expectedBuilder1.Metadata.Health.DataLinks, should.Resemble(map[string]string{
				"user": "data-link-for-amd-cq-from-req",
			}))
			assert.Loosely(t, expectedBuilder1.Metadata.Health.DocLinks, should.Resemble(map[string]string{
				"user": "doc-link-for-amd-cq-from-req",
			}))
			assert.Loosely(t, expectedBuilder2.Metadata.Health.HealthScore, should.Equal(8))
			assert.Loosely(t, expectedBuilder2.Metadata.Health.Reporter, should.Equal("someone@example.com"))
			assert.Loosely(t, expectedBuilder2.Metadata.Health.ReportedTime, should.NotBeNil)
			assert.Loosely(t, expectedBuilder2.Metadata.Health.DataLinks, should.Resemble(map[string]string{
				"user": "data-link-for-amd-cq-2",
			}))
			assert.Loosely(t, expectedBuilder2.Metadata.Health.DocLinks, should.Resemble(map[string]string{
				"user": "doc-link-for-amd-cq-2",
			}))
			assert.Loosely(t, expectedBuilder3.Metadata.Health.HealthScore, should.Equal(2))
			assert.Loosely(t, expectedBuilder3.Metadata.Health.Reporter, should.Equal("someone@example.com"))
			assert.Loosely(t, expectedBuilder3.Metadata.Health.ReportedTime, should.NotBeNil)
		})

		t.Run("one builder does not exist", func(t *ftt.Test) {
			bktKey := model.BucketKey(ctx, "chrome", "cq")
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: bktKey,
			}), should.BeNil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chrome:cq", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:amd-cq", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq",
						},
						Health: &pb.HealthStatus{
							HealthScore: 9,
						},
					},
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq-2",
						},
						Health: &pb.HealthStatus{
							HealthScore: 8,
						},
					},
				},
			}
			resp, err := srv.SetBuilderHealth(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Resemble(&pb.SetBuilderHealthResponse{
				Responses: []*pb.SetBuilderHealthResponse_Response{
					{
						Response: &pb.SetBuilderHealthResponse_Response_Result{
							Result: &emptypb.Empty{},
						},
					},
					{
						Response: &pb.SetBuilderHealthResponse_Response_Error{
							Error: &status.Status{
								Code:    13,
								Message: "attaching a status: rpc error: code = Internal desc = failed to get builder amd-cq-2: datastore: no such entity",
							},
						},
					},
				},
			}))
			expectedBuilder1 := &model.Builder{ID: "amd-cq", Parent: bktKey}
			assert.Loosely(t, datastore.Get(ctx, expectedBuilder1), should.BeNil)
			assert.Loosely(t, expectedBuilder1.Metadata.Health.HealthScore, should.Equal(9))
		})

		t.Run("multiple requests for same builder", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: model.BucketKey(ctx, "chrome", "cq"),
			}), should.BeNil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chrome:cq", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:amd-cq", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq",
						},
						Health: &pb.HealthStatus{HealthScore: 12},
					},
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq",
						},
						Health: &pb.HealthStatus{HealthScore: 13},
					},
				},
			}
			_, err := srv.SetBuilderHealth(ctx, req)
			assert.Loosely(t, err.Error(), should.ContainSubstring("The following builder has multiple entries: chrome/cq/amd-cq"))
		})
	})

	ftt.Run("links", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "fake-cr-buildbucket")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)
		srv := &Builders{}
		testutil.PutBucket(ctx, "chrome", "cq", nil)
		bktKey := model.BucketKey(ctx, "chrome", "cq")
		assert.Loosely(t, datastore.Put(ctx, &model.Builder{
			ID:     "amd-cq",
			Parent: bktKey,
			Config: &pb.BuilderConfig{
				BuilderHealthMetricsLinks: &pb.BuilderConfig_BuilderHealthLinks{
					DataLinks: map[string]string{
						"google.com":   "go/somelink",
						"chromium.org": "some_public_link.com",
					},
					DocLinks: map[string]string{
						"google.com":   "go/some_doc_link",
						"chromium.org": "some_public_doc_link.com",
					},
				},
			},
		}), should.BeNil)

		t.Run("links from cfg", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@google.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chrome:cq", Permission: bbperms.BuildersSetHealth},
					{Realm: "builder:amd-cq", Permission: bbperms.BuildersSetHealth},
				},
			})
			req := &pb.SetBuilderHealthRequest{
				Health: []*pb.SetBuilderHealthRequest_BuilderHealth{
					{
						Id: &pb.BuilderID{
							Project: "chrome",
							Bucket:  "cq",
							Builder: "amd-cq",
						},
						Health: &pb.HealthStatus{
							HealthScore: 9,
						},
					},
				},
			}
			_, err := srv.SetBuilderHealth(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			expectedBuilder := &model.Builder{ID: "amd-cq", Parent: bktKey}
			assert.Loosely(t, datastore.Get(ctx, expectedBuilder), should.BeNil)
			assert.Loosely(t, expectedBuilder.Metadata.Health.DataLinks, should.Resemble(map[string]string{
				"google.com":   "go/somelink",
				"chromium.org": "some_public_link.com",
			}))
			assert.Loosely(t, expectedBuilder.Metadata.Health.DocLinks, should.Resemble(map[string]string{
				"google.com":   "go/some_doc_link",
				"chromium.org": "some_public_doc_link.com",
			}))
		})
	})
}
