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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/buildbucket"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestSendBatchReq(t *testing.T) {
	ftt.Run("SendBatchReq", t, func(t *ftt.Test) {
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

		t.Run("success", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(expectedRes))
		})

		t.Run("sub-requests transient errors", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(expectedRes))
		})
	})
	ftt.Run("updateRequest", t, func(t *ftt.Test) {
		ctx := context.Background()
		t.Run("updateRequest if dummy token", func(t *ftt.Test) {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{
							CanOutliveParent: pb.Trinary_YES,
						},
					}},
				},
			}
			updateRequest(ctx, req, buildbucket.DummyBuildbucketToken)
			assert.Loosely(t, req.Requests[0].GetScheduleBuild().CanOutliveParent, should.Equal(pb.Trinary_UNSET))
		})
		t.Run("updateRequest if empty token", func(t *ftt.Test) {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{
							CanOutliveParent: pb.Trinary_YES,
						},
					}},
				},
			}
			updateRequest(ctx, req, "")
			assert.Loosely(t, req.Requests[0].GetScheduleBuild().CanOutliveParent, should.Equal(pb.Trinary_YES))
		})
		t.Run("updateRequest if real token", func(t *ftt.Test) {
			req := &pb.BatchRequest{
				Requests: []*pb.BatchRequest_Request{
					{Request: &pb.BatchRequest_Request_ScheduleBuild{
						ScheduleBuild: &pb.ScheduleBuildRequest{
							CanOutliveParent: pb.Trinary_YES,
						},
					}},
				},
			}
			updateRequest(ctx, req, "real token")
			assert.Loosely(t, req.Requests[0].GetScheduleBuild().CanOutliveParent, should.Equal(pb.Trinary_YES))
		})
	})
}
