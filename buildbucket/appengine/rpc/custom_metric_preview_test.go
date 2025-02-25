// Copyright 2024 The LUCI Authors.
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

func TestCustomMetricPreview(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	ftt.Run("CustomMetricPreview", t, func(t *ftt.Test) {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
		})

		t.Run("validation", func(t *ftt.Test) {
			t.Run("empty request", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.ErrLike(".build_id: required"))
				assert.Loosely(t, rsp, should.BeNil)
			})
			t.Run("no metric", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.ErrLike(".metric_definition: required"))
				assert.Loosely(t, rsp, should.BeNil)
			})
			t.Run("no base", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
					MetricDefinition: &pb.CustomMetricDefinition{
						Name:       "chrome/infra/custom/builds/started",
						Predicates: []string{`build.tags.get_value("os")!=""`},
					},
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.ErrLike("metric_base is required"))
				assert.Loosely(t, rsp, should.BeNil)
			})
			t.Run("metric has no predicates", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
					MetricDefinition: &pb.CustomMetricDefinition{
						Name: "chrome/infra/custom/builds/started",
					},
					Class: &pb.CustomMetricPreviewRequest_MetricBase{
						MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED,
					},
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.ErrLike("metric_definition.predicates is required"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("builder metric has extra_fields", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
					MetricDefinition: &pb.CustomMetricDefinition{
						Name:       "chrome/infra/custom/builds/count",
						Predicates: []string{`build.tags.get_value("os")!=""`},
						ExtraFields: map[string]string{
							"os": `"os"`,
						},
					},
					Class: &pb.CustomMetricPreviewRequest_MetricBase{
						MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
					},
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`custom builder metric cannot have extra_fields`))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("metric reuses base fields in extra_fields", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
					MetricDefinition: &pb.CustomMetricDefinition{
						Name:       "chrome/infra/custom/builds/completed",
						Predicates: []string{`build.tags.get_value("os")!=""`},
						ExtraFields: map[string]string{
							"status": `"status"`,
						},
					},
					Class: &pb.CustomMetricPreviewRequest_MetricBase{
						MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED,
					},
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`cannot contain base fields ["status"] in extra_fields`))
				assert.Loosely(t, rsp, should.BeNil)
			})

			req := &pb.CustomMetricPreviewRequest{
				BuildId: 1,
				MetricDefinition: &pb.CustomMetricDefinition{
					Name:       "chrome/infra/custom/builds/started",
					Predicates: []string{`build.tags.get_value("os")!=""`},
				},
				Class: &pb.CustomMetricPreviewRequest_MetricBase{
					MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED,
				},
			}
			t.Run("build not found", func(t *ftt.Test) {
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.ErrLike("not found"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("with build", func(t *ftt.Test) {
				testutil.PutBucket(ctx, "project", "bucket", nil)
				build := &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}
				assert.Loosely(t, datastore.Put(ctx, build), should.BeNil)
				t.Run("no access", func(t *ftt.Test) {
					rsp, err := srv.CustomMetricPreview(ctx, req)
					assert.Loosely(t, err, should.ErrLike("not found"))
					assert.Loosely(t, rsp, should.BeNil)
				})
				t.Run("OK", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						),
					})
					rsp, err := srv.CustomMetricPreview(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp, should.NotBeNil)
				})
			})
		})

		t.Run("preview", func(t *ftt.Test) {
			testutil.PutBucket(ctx, "project", "bucket", nil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
				),
			})

			build := &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_FAILURE,
					Tags: []*pb.StringPair{
						{
							Key:   "os",
							Value: "mac",
						},
					},
				},
				Tags: []string{
					"os:mac",
				},
			}
			assert.Loosely(t, datastore.Put(ctx, build), should.BeNil)

			t.Run("invalid predicate", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
					MetricDefinition: &pb.CustomMetricDefinition{
						Name: "chrome/infra/custom/builds/completed",
						// not a bool expression
						Predicates: []string{`build.tags.get_value("os")`},
					},
					Class: &pb.CustomMetricPreviewRequest_MetricBase{
						MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED,
					},
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expected := &pb.CustomMetricPreviewResponse{
					Response: &pb.CustomMetricPreviewResponse_Error{
						Error: "failed to evaluate the build with predicates: failed to generate CEL expression: expect bool, got string",
					},
				}
				assert.Loosely(t, rsp, should.Match(expected))
			})

			t.Run("build not pass predicate", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
					MetricDefinition: &pb.CustomMetricDefinition{
						Name:       "chrome/infra/custom/builds/completed",
						Predicates: []string{`build.tags.get_value("os")=="linux"`},
					},
					Class: &pb.CustomMetricPreviewRequest_MetricBase{
						MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED,
					},
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expected := &pb.CustomMetricPreviewResponse{
					Response: &pb.CustomMetricPreviewResponse_Error{
						Error: "the build doesn't pass the predicates evaluation, it will not be reported by the custom metric",
					},
				}
				assert.Loosely(t, rsp, should.Match(expected))
			})

			t.Run("invalid field", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
					MetricDefinition: &pb.CustomMetricDefinition{
						Name:       "chrome/infra/custom/builds/completed",
						Predicates: []string{`build.tags.exists(t, t.key=="os")`},
						ExtraFields: map[string]string{
							// Not a string expression
							"os": `build.tags.get_value("os")==""`,
						},
					},
					Class: &pb.CustomMetricPreviewRequest_MetricBase{
						MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED,
					},
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expected := &pb.CustomMetricPreviewResponse{
					Response: &pb.CustomMetricPreviewResponse_Error{
						Error: "failed to evaluate the build with extra_fields: failed to generate CEL expression: expect string, got bool",
					},
				}
				assert.Loosely(t, rsp, should.Match(expected))
			})

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
					MetricDefinition: &pb.CustomMetricDefinition{
						Name:       "chrome/infra/custom/builds/completed",
						Predicates: []string{`build.tags.exists(t, t.key=="os")`},
						ExtraFields: map[string]string{
							"os": `build.tags.get_value("os")`,
						},
					},
					Class: &pb.CustomMetricPreviewRequest_MetricBase{
						MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED,
					},
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expected := &pb.CustomMetricPreviewResponse{
					Response: &pb.CustomMetricPreviewResponse_Report_{
						Report: &pb.CustomMetricPreviewResponse_Report{
							Fields: []*pb.StringPair{
								{Key: "status", Value: "FAILURE"},
								{Key: "os", Value: "mac"},
							},
						},
					},
				}
				assert.Loosely(t, rsp, should.Match(expected))
			})
		})
	})
}
