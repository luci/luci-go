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

	"go.chromium.org/luci/gae/filter/txndefer"
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

func TestValidateSetBuilderHealthRequest(t *testing.T) {
	t.Parallel()
	Convey("validateSetBuilderHealthRequest", t, func() {
		ctx := memory.Use(context.Background())
		testutil.PutBucket(ctx, "project", "bucket", nil)

		Convey("empty req", func() {
			req := &pb.SetBuilderHealthRequest{}
			err := validateRequest(ctx, req, nil, nil)
			So(err, ShouldBeNil)
		})

		Convey("ok req", func() {
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
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, []*pb.SetBuilderHealthResponse_Response{
				nil, nil, nil,
			})
		})

		Convey("miltiple entries", func() {
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
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "The following builder has multiple entries: project/bucket/builder")
		})

		Convey("bad health score", func() {
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
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, []*pb.SetBuilderHealthResponse_Response{
				{
					Response: &pb.SetBuilderHealthResponse_Response_Error{
						Error: &status.Status{
							Code:    3,
							Message: "Builder: project/bucket/builder: HealthScore should be between 0 and 10",
						},
					},
				},
			})
		})

		Convey("builderID not present, health is", func() {
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
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, ".health[0].id: required")
		})
	})
}

func TestSetBuilderHealth(t *testing.T) {
	t.Parallel()

	Convey("requests", t, func() {
		ctx := memory.Use(context.Background())
		srv := &Builders{}
		testutil.PutBucket(ctx, "chrome", "cq", nil)
		testutil.PutBucket(ctx, "chromeos", "cq", nil)

		Convey("bad request; no perms", func() {
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
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &pb.SetBuilderHealthResponse{
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
			})
		})

		Convey("bad request; has perms", func() {
			So(datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: model.BucketKey(ctx, "chrome", "cq"),
			}), ShouldBeNil)
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
			So(err.Error(), ShouldContainSubstring, ".health[1].id.project: required (and 2 other errors)")
		})

		Convey("bad req; one no perm, one validation err, one no builder saved", func() {
			So(datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: model.BucketKey(ctx, "chrome", "cq"),
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: model.BucketKey(ctx, "chromeos", "cq"),
			}), ShouldBeNil)
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
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &pb.SetBuilderHealthResponse{
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
			})
		})
	})

	Convey("existing entities", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "fake-cr-buildbucket")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)
		srv := &Builders{}
		testutil.PutBucket(ctx, "chrome", "cq", nil)

		Convey("builders exists; update is normal", func() {
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
			So(datastore.Put(ctx, bldrToPut1, bldrToPut2, bldrToPut3), ShouldBeNil)
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
			So(err, ShouldBeNil)
			expectedBuilder1 := &model.Builder{ID: "amd-cq", Parent: bktKey}
			expectedBuilder2 := &model.Builder{ID: "amd-cq-2", Parent: bktKey}
			expectedBuilder3 := &model.Builder{ID: "amd-cq-3", Parent: bktKey}
			So(datastore.Get(ctx, expectedBuilder1, expectedBuilder2, expectedBuilder3), ShouldBeNil)
			So(expectedBuilder1.Metadata.Health.HealthScore, ShouldEqual, 9)
			So(expectedBuilder1.Metadata.Health.Reporter, ShouldEqual, "someone@example.com")
			So(expectedBuilder1.Metadata.Health.ReportedTime, ShouldNotBeNil)
			So(expectedBuilder1.Metadata.Health.DataLinks, ShouldResemble, map[string]string{
				"user": "data-link-for-amd-cq-from-req",
			})
			So(expectedBuilder1.Metadata.Health.DocLinks, ShouldResemble, map[string]string{
				"user": "doc-link-for-amd-cq-from-req",
			})
			So(expectedBuilder2.Metadata.Health.HealthScore, ShouldEqual, 8)
			So(expectedBuilder2.Metadata.Health.Reporter, ShouldEqual, "someone@example.com")
			So(expectedBuilder2.Metadata.Health.ReportedTime, ShouldNotBeNil)
			So(expectedBuilder2.Metadata.Health.DataLinks, ShouldResemble, map[string]string{
				"user": "data-link-for-amd-cq-2",
			})
			So(expectedBuilder2.Metadata.Health.DocLinks, ShouldResemble, map[string]string{
				"user": "doc-link-for-amd-cq-2",
			})
			So(expectedBuilder3.Metadata.Health.HealthScore, ShouldEqual, 2)
			So(expectedBuilder3.Metadata.Health.Reporter, ShouldEqual, "someone@example.com")
			So(expectedBuilder3.Metadata.Health.ReportedTime, ShouldNotBeNil)
		})

		Convey("one builder does not exist", func() {
			bktKey := model.BucketKey(ctx, "chrome", "cq")
			So(datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: bktKey,
			}), ShouldBeNil)
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
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &pb.SetBuilderHealthResponse{
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
			})
			expectedBuilder1 := &model.Builder{ID: "amd-cq", Parent: bktKey}
			So(datastore.Get(ctx, expectedBuilder1), ShouldBeNil)
			So(expectedBuilder1.Metadata.Health.HealthScore, ShouldEqual, 9)
		})

		Convey("multiple requests for same builder", func() {
			So(datastore.Put(ctx, &model.Builder{
				ID:     "amd-cq",
				Parent: model.BucketKey(ctx, "chrome", "cq"),
			}), ShouldBeNil)
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
			So(err.Error(), ShouldContainSubstring, "The following builder has multiple entries: chrome/cq/amd-cq")
		})
	})

	Convey("links", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "fake-cr-buildbucket")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)
		srv := &Builders{}
		testutil.PutBucket(ctx, "chrome", "cq", nil)
		bktKey := model.BucketKey(ctx, "chrome", "cq")
		So(datastore.Put(ctx, &model.Builder{
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
		}), ShouldBeNil)

		Convey("links from cfg", func() {
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
			So(err, ShouldBeNil)
			expectedBuilder := &model.Builder{ID: "amd-cq", Parent: bktKey}
			So(datastore.Get(ctx, expectedBuilder), ShouldBeNil)
			So(expectedBuilder.Metadata.Health.DataLinks, ShouldResemble, map[string]string{
				"google.com":   "go/somelink",
				"chromium.org": "some_public_link.com",
			})
			So(expectedBuilder.Metadata.Health.DocLinks, ShouldResemble, map[string]string{
				"google.com":   "go/some_doc_link",
				"chromium.org": "some_public_doc_link.com",
			})
		})
	})
}
