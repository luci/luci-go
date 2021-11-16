// Copyright 2019 The LUCI Authors.
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

package cli

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/proto"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSendBatchReq(t *testing.T) {
	Convey("SendBatchReq", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockBBClient := pb.NewMockBuildsClient(ctl)
		ctx := context.Background()

		build := &pb.Build{
			Id: 1,
			Builder: &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder1",
			},
			Input: &pb.Build_Input{},
		}

		Convey("success", func() {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{},
					}},
				},
			}
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_ScheduleBuild{
						ScheduleBuild: build,
					}},
				},
			}
			mockBBClient.EXPECT().Batch(ctx, req).Return(expectedRes, nil)
			res, err := sendBatchReq(ctx, req, mockBBClient)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})

		Convey("sub-requests transient errors", func() {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{},
					}},
					{Request: &pb.BatchRequest_Request_SearchBuilds{
						SearchBuilds: &pb.SearchBuildsRequest{},
					}},
					{Request: &pb.BatchRequest_Request_GetBuild{
						GetBuild: &pb.GetBuildRequest{Id: 1},
					}},
				},
			}
			expectedRes := &pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_ScheduleBuild{
						ScheduleBuild: build,
					}},
					{Response: &pb.BatchResponse_Response_SearchBuilds{
						SearchBuilds: &pb.SearchBuildsResponse{
							Builds: []*pb.Build{build},
						},
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: status.New(codes.InvalidArgument, "bad request").Proto(),
					}},
				},
			}

			// first call
			mockBBClient.EXPECT().Batch(ctx, req).Times(1).Return(&pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_ScheduleBuild{
						ScheduleBuild: build,
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: status.New(codes.Internal, "Internal server error").Proto(),
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: status.New(codes.Internal, "Internal server error").Proto(),
					}},
				},
			}, nil)
			// retry the 2nd and 3rd sub-requests
			mockBBClient.EXPECT().Batch(ctx, proto.MatcherEqual(
				&pb.BatchRequest{
					Requests: []*pb.BatchRequest_Request{
						{Request: &pb.BatchRequest_Request_SearchBuilds{
							SearchBuilds: &pb.SearchBuildsRequest{},
						}},
						{Request: &pb.BatchRequest_Request_GetBuild{
							GetBuild: &pb.GetBuildRequest{Id: 1},
						}},
					},
				})).Times(1).Return(&pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_Error{
						Error: status.New(codes.DeadlineExceeded, "timeout").Proto(),
					}},
					{Response: &pb.BatchResponse_Response_Error{
						Error: status.New(codes.InvalidArgument, "bad request").Proto(),
					}},
				},
			}, nil)
			// retry the 2nd sub-request again
			mockBBClient.EXPECT().Batch(ctx, proto.MatcherEqual(
				&pb.BatchRequest{
					Requests: []*pb.BatchRequest_Request{
						{Request: &pb.BatchRequest_Request_SearchBuilds{
							SearchBuilds: &pb.SearchBuildsRequest{},
						}},
					},
				})).Times(1).Return(&pb.BatchResponse{
				Responses: []*pb.BatchResponse_Response{
					{Response: &pb.BatchResponse_Response_SearchBuilds{
						SearchBuilds: &pb.SearchBuildsResponse{
							Builds: []*pb.Build{build},
						},
					}},
				},
			}, nil)
			res, err := sendBatchReq(ctx, req, mockBBClient)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expectedRes)
		})
	})
}
