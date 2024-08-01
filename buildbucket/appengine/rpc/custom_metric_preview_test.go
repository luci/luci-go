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

func TestCustomMetricPreview(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	Convey("CustomMetricPreview", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
		})

		Convey("validation", func() {
			Convey("empty request", func() {
				req := &pb.CustomMetricPreviewRequest{}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				So(err, ShouldErrLike, ".build_id: required")
				So(rsp, ShouldBeNil)
			})
			Convey("no metric", func() {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				So(err, ShouldErrLike, ".metric_definition: required")
				So(rsp, ShouldBeNil)
			})
			Convey("no base", func() {
				req := &pb.CustomMetricPreviewRequest{
					BuildId: 1,
					MetricDefinition: &pb.CustomMetricDefinition{
						Name:       "chrome/infra/custom/builds/started",
						Predicates: []string{`build.tags.get_value("os")!=""`},
					},
				}
				rsp, err := srv.CustomMetricPreview(ctx, req)
				So(err, ShouldErrLike, "metric_base is required")
				So(rsp, ShouldBeNil)
			})
			Convey("metric has no predicates", func() {
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
				So(err, ShouldErrLike, "metric_definition.predicates is required")
				So(rsp, ShouldBeNil)
			})
			Convey("metric reuses base fields in extra_fields", func() {
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
				So(err, ShouldErrLike, `base fields ["status"] cannot be used in extra_fields`)
				So(rsp, ShouldBeNil)
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
			Convey("build not found", func() {
				rsp, err := srv.CustomMetricPreview(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
			})

			Convey("with build", func() {
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
				So(datastore.Put(ctx, build), ShouldBeNil)
				Convey("no access", func() {
					rsp, err := srv.CustomMetricPreview(ctx, req)
					So(err, ShouldErrLike, "not found")
					So(rsp, ShouldBeNil)
				})
				Convey("OK", func() {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						),
					})
					rsp, err := srv.CustomMetricPreview(ctx, req)
					So(err, ShouldBeNil)
					So(rsp, ShouldNotBeNil)
				})
			})
		})

		Convey("preview", func() {
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
			So(datastore.Put(ctx, build), ShouldBeNil)

			Convey("invalid predicate", func() {
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
				So(err, ShouldBeNil)
				expected := &pb.CustomMetricPreviewResponse{
					Response: &pb.CustomMetricPreviewResponse_Error{
						Error: "failed to evaluate the build with predicates: failed to generate CEL expression: expect bool, got string",
					},
				}
				So(rsp, ShouldResembleProto, expected)
			})

			Convey("build not pass predicate", func() {
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
				So(err, ShouldBeNil)
				expected := &pb.CustomMetricPreviewResponse{
					Response: &pb.CustomMetricPreviewResponse_Error{
						Error: "the build doesn't pass the predicates evaluation, it will not be reported by the custom metric",
					},
				}
				So(rsp, ShouldResembleProto, expected)
			})

			Convey("invalid field", func() {
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
				So(err, ShouldBeNil)
				expected := &pb.CustomMetricPreviewResponse{
					Response: &pb.CustomMetricPreviewResponse_Error{
						Error: "failed to evaluate the build with extra_fields: failed to generate CEL expression: expect string, got bool",
					},
				}
				So(rsp, ShouldResembleProto, expected)
			})

			Convey("ok", func() {
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
				So(err, ShouldBeNil)
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
				So(rsp, ShouldResembleProto, expected)
			})
		})
	})
}
