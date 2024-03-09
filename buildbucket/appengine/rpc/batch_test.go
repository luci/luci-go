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
	"math/rand"
	"testing"

	"github.com/golang/mock/gomock"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/bqlog"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBatch(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:caller@example.com")

	Convey("Batch", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		srv := &Builds{}
		ctx, _ := tq.TestingContext(txndefer.FilterRDS(memory.Use(context.Background())), nil)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		So(config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{}), ShouldBeNil)

		b := &bqlog.Bundler{
			CloudProject: "project",
			Dataset:      "dataset",
		}
		ctx = withBundler(ctx, b)
		b.RegisterSink(bqlog.Sink{
			Prototype: &pb.PRPCRequestLog{},
			Table:     "table",
		})
		b.Start(ctx, &bqlog.FakeBigQueryWriter{})
		defer b.Shutdown(ctx)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsCancel),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
			),
		})
		So(datastore.Put(
			ctx,
			&model.Bucket{
				ID:     "bucket",
				Parent: model.ProjectKey(ctx, "project"),
				Proto:  &pb.Bucket{},
			},
			&model.Bucket{
				ID:     "bucket1",
				Parent: model.ProjectKey(ctx, "project"),
				Proto: &pb.Bucket{
					Shadow: "bucket1",
				},
			},
			&model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder1",
					},
				},
			},
			&model.Build{
				Proto: &pb.Build{
					Id: 2,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder2",
					},
				},
			}), ShouldBeNil)

		Convey("empty", func() {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{},
			}
			res, err := srv.Batch(ctx, req)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.BatchResponse{})

			req = &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{{}},
			}
			res, err = srv.Batch(ctx, req)
			So(err, ShouldNotBeNil)
			So(res, ShouldBeNil)
			So(err, ShouldErrLike, "request includes an unsupported type")
		})

		Convey("error", func() {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_GetBuild{
						GetBuild: &pb.GetBuildRequest{BuildNumber: 1},
					}},
				},
			}
			res, err := srv.Batch(ctx, req)
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_Error{
						Error: &spb.Status{
							Code:    3,
							Message: "bad request: one of id or (builder and build_number) is required",
						},
					}},
				},
			}
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})

		Convey("getBuildStatus req", func() {
			bs := &model.BuildStatus{
				Build:        datastore.MakeKey(ctx, "Build", 500),
				BuildAddress: "project/bucket/builder/b500",
				Status:       pb.Status_SCHEDULED,
			}
			So(datastore.Put(ctx, bs), ShouldBeNil)
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_GetBuildStatus{
						GetBuildStatus: &pb.GetBuildStatusRequest{Id: 1},
					}},
					{Request: &pb.BatchRequest_Request_GetBuildStatus{
						GetBuildStatus: &pb.GetBuildStatusRequest{Id: 500},
					}},
				},
			}
			res, err := srv.Batch(ctx, req)
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_GetBuildStatus{
						GetBuildStatus: &pb.Build{
							Id:     1,
							Status: pb.Status_STATUS_UNSPECIFIED,
						},
					}},
					{Response: &pb.BatchResponse_Response_GetBuildStatus{
						GetBuildStatus: &pb.Build{
							Id:     500,
							Status: pb.Status_SCHEDULED,
						},
					}},
				},
			}
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})

		Convey("getBuild req", func() {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_GetBuild{
						GetBuild: &pb.GetBuildRequest{Id: 1},
					}},
				},
			}
			res, err := srv.Batch(ctx, req)
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_GetBuild{
						GetBuild: &pb.Build{
							Id: 1,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder1",
							},
							Input: &pb.Build_Input{},
						},
					}},
				},
			}
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})

		Convey("searchBuilds req", func() {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_SearchBuilds{
						SearchBuilds: &pb.SearchBuildsRequest{},
					}},
				},
			}
			res, err := srv.Batch(ctx, req)
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_SearchBuilds{
						SearchBuilds: &pb.SearchBuildsResponse{
							Builds: []*pb.Build{
								{Id: 1,
									Builder: &pb.BuilderID{
										Project: "project",
										Bucket:  "bucket",
										Builder: "builder1",
									},
									Input: &pb.Build_Input{},
								},
								{Id: 2,
									Builder: &pb.BuilderID{
										Project: "project",
										Bucket:  "bucket",
										Builder: "builder2",
									},
									Input: &pb.Build_Input{},
								},
							},
						},
					}},
				},
			}
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})

		Convey("get and search reqs", func() {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_GetBuild{
						GetBuild: &pb.GetBuildRequest{Id: 1},
					}},
					{Request: &pb.BatchRequest_Request_SearchBuilds{
						SearchBuilds: &pb.SearchBuildsRequest{},
					}},
					{Request: &pb.BatchRequest_Request_GetBuild{
						GetBuild: &pb.GetBuildRequest{Id: 2},
					}},
				},
			}
			res, err := srv.Batch(ctx, req)
			build1 := &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder1",
				},
				Input: &pb.Build_Input{},
			}
			build2 := &pb.Build{
				Id: 2,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder2",
				},
				Input: &pb.Build_Input{},
			}
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_GetBuild{
						GetBuild: build1,
					}},
					{Response: &pb.BatchResponse_Response_SearchBuilds{
						SearchBuilds: &pb.SearchBuildsResponse{
							Builds: []*pb.Build{build1, build2},
						},
					}},
					{Response: &pb.BatchResponse_Response_GetBuild{
						GetBuild: build2,
					}},
				},
			}
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})

		Convey("schedule req", func() {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{},
					}},
				},
			}
			res, err := srv.Batch(ctx, req)
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_Error{
						Error: &spb.Status{
							Code:    3,
							Message: "bad request: builder or template_build_id is required",
						},
					}},
				},
			}
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})

		Convey("schedule batch", func() {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{},
					}},
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
							},
						},
					}},
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
							},
						},
					}},
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							ShadowInput: &pb.ScheduleBuildRequest_ShadowInput{},
						},
					}},
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket1",
								Builder: "builder",
							},
							ShadowInput: &pb.ScheduleBuildRequest_ShadowInput{},
						},
					}},
				},
			}
			res, err := srv.Batch(ctx, req)
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_Error{
						Error: &spb.Status{
							Code:    3,
							Message: "bad request: builder or template_build_id is required",
						},
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: &spb.Status{
							Code:    3,
							Message: "bad request: builder: bucket is required",
						},
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: &spb.Status{
							Code:    3,
							Message: "bad request: builder: builder is required",
						},
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: &spb.Status{
							Code:    3,
							Message: "bad request: scheduling a shadow build in the original bucket is not allowed",
						},
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: &spb.Status{
							Code:    3,
							Message: "bad request: scheduling a shadow build in the original bucket is not allowed",
						},
					}},
				},
			}
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})

		Convey("cancel req", func() {
			now := testclock.TestRecentTimeLocal
			ctx, _ = testclock.UseTime(ctx, now)
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_CancelBuild{
						CancelBuild: &pb.CancelBuildRequest{
							Id:              1,
							SummaryMarkdown: "summary",
							Mask: &pb.BuildMask{
								Fields: &fieldmaskpb.FieldMask{
									Paths: []string{
										"id",
										"builder",
										"update_time",
										"cancel_time",
										"status",
										"cancellation_markdown",
									},
								},
							},
						},
					}},
				},
			}
			res, err := srv.Batch(ctx, req)
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_CancelBuild{
						CancelBuild: &pb.Build{
							Id: 1,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder1",
							},
							UpdateTime:           timestamppb.New(now),
							CancelTime:           timestamppb.New(now),
							CancellationMarkdown: "summary",
						},
					}},
				},
			}
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})

		Convey("get, schedule, search, cancel and get_build_status in req", func() {
			req := &pb.BatchRequest{}
			err := protojson.Unmarshal([]byte(`{
				"requests": [
					{"getBuild": {"id": "1"}},
					{"scheduleBuild": {}},
					{"searchBuilds": {}},
					{"cancelBuild": {}},
					{"getBuildStatus": {"id": "1"}}
				]}`), req)
			So(err, ShouldBeNil)
			expectedPyReq := &pb.BatchRequest{}
			err = protojson.Unmarshal([]byte(`{
				"requests": [
					{"scheduleBuild": {}}
				]}`), expectedPyReq)
			So(err, ShouldBeNil)
			actualRes, err := srv.Batch(ctx, req)
			build1 := &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder1",
				},
				Input: &pb.Build_Input{},
			}
			build2 := &pb.Build{
				Id: 2,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder2",
				},
				Input: &pb.Build_Input{},
			}
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_GetBuild{
						GetBuild: build1,
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: &spb.Status{
							Code:    3,
							Message: "bad request: builder or template_build_id is required",
						},
					}},
					{Response: &pb.BatchResponse_Response_SearchBuilds{
						SearchBuilds: &pb.SearchBuildsResponse{
							Builds: []*pb.Build{build1, build2},
						},
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: &spb.Status{
							Code:    3,
							Message: "bad request: id is required",
						},
					}},
					{Response: &pb.BatchResponse_Response_GetBuildStatus{
						GetBuildStatus: &pb.Build{
							Id:     1,
							Status: pb.Status_STATUS_UNSPECIFIED,
						},
					}},
				},
			}
			So(err, ShouldBeNil)
			So(actualRes, ShouldResembleProto, expectedRes)
		})

		Convey("exceed max read reqs amount", func() {
			req := &pb.BatchRequest{}
			for i := 0; i < readReqsSizeLimit+1; i++ {
				req.Requests = append(req.Requests, &pb.BatchRequest_Request{Request: &pb.BatchRequest_Request_GetBuild{}})
			}
			_, err := srv.Batch(ctx, req)
			So(err, ShouldErrLike, "the maximum allowed read request count in Batch is 1000.")
		})

		Convey("exceed max write reqs amount", func() {
			req := &pb.BatchRequest{}
			for i := 0; i < writeReqsSizeLimit+1; i++ {
				req.Requests = append(req.Requests, &pb.BatchRequest_Request{Request: &pb.BatchRequest_Request_ScheduleBuild{}})
			}
			_, err := srv.Batch(ctx, req)
			So(err, ShouldErrLike, "the maximum allowed write request count in Batch is 200.")
		})
	})
}
