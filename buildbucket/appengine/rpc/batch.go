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
	"fmt"

	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	readReqsSizeLimit  = 1000
	writeReqsSizeLimit = 200
)

// Batch handles a batch request. Implements pb.BuildsServer.
func (b *Builds) Batch(ctx context.Context, req *pb.BatchRequest) (*pb.BatchResponse, error) {
	res := &pb.BatchResponse{}
	if len(req.GetRequests()) == 0 {
		return res, nil
	}
	res.Responses = make([]*pb.BatchResponse_Response, len(req.Requests))

	scheduleReqs := 0
	for _, r := range req.Requests {
		switch r.Request.(type) {
		case *pb.BatchRequest_Request_ScheduleBuild:
			scheduleReqs++
		case *pb.BatchRequest_Request_GetBuild, *pb.BatchRequest_Request_SearchBuilds, *pb.BatchRequest_Request_CancelBuild:
		default:
			return nil, appstatus.BadRequest(errors.New("request includes an unsupported type"))
		}
	}

	// validate request count
	if scheduleReqs > writeReqsSizeLimit {
		return nil, appstatus.BadRequest(errors.Reason("the maximum allowed schedule request count in Batch is %d.", writeReqsSizeLimit).Err())
	}
	if len(req.Requests)-scheduleReqs > readReqsSizeLimit {
		return nil, appstatus.BadRequest(errors.Reason("the maximum allowed get+search+cancel request count in Batch is %d.", readReqsSizeLimit).Err())
	}

	err := parallel.WorkPool(64, func(c chan<- func() error) {
		for i, r := range req.Requests {
			i, r := i, r
			c <- func() error {
				response := &pb.BatchResponse_Response{}
				var err error
				switch r.Request.(type) {
				case *pb.BatchRequest_Request_GetBuild:
					ret, e := b.GetBuild(ctx, r.GetGetBuild())
					response.Response = &pb.BatchResponse_Response_GetBuild{GetBuild: ret}
					err = e
				case *pb.BatchRequest_Request_SearchBuilds:
					ret, e := b.SearchBuilds(ctx, r.GetSearchBuilds())
					response.Response = &pb.BatchResponse_Response_SearchBuilds{SearchBuilds: ret}
					err = e
				case *pb.BatchRequest_Request_CancelBuild:
					ret, e := b.CancelBuild(ctx, r.GetCancelBuild())
					response.Response = &pb.BatchResponse_Response_CancelBuild{CancelBuild: ret}
					err = e
				case *pb.BatchRequest_Request_ScheduleBuild:
					ret, e := b.ScheduleBuild(ctx, r.GetScheduleBuild())
					response.Response = &pb.BatchResponse_Response_ScheduleBuild{ScheduleBuild: ret}
					err = e
				default:
					panic(fmt.Sprintf("attempted to handle unexpected request type %T", r.Request))
				}
				if err != nil {
					logging.Warningf(ctx, "Error from Go: %s", err)
					if goErrSt, ok := convertGRPCError(err); ok {
						return appstatus.Error(goErrSt.Code(), goErrSt.Message())
					}
					response.Response = toBatchResponseError(ctx, err)
				}
				res.Responses[i] = response
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}

// convertGRPCError converts to a grpc Status, if this error is a grpc error.
//
// If it's DeadlineExceeded error, return a Status with the internal error code
// as a short-term solution (crbug.com/1174310) for the caller side retry, e.g., bb cli.
//
// If it's not a grpc error, ok is false and a Status is returned with
// codes.Unknown and the original error message.
func convertGRPCError(err error) (*grpcStatus.Status, bool) {
	gStatus, ok := grpcStatus.FromError(err)
	if !ok {
		return gStatus, false
	}
	if gStatus.Code() == codes.DeadlineExceeded {
		return grpcStatus.New(codes.Internal, gStatus.Message()), true
	}
	return gStatus, true
}

// toBatchResponseError converts an error to BatchResponse_Response_Error type.
func toBatchResponseError(ctx context.Context, err error) *pb.BatchResponse_Response_Error {
	st, ok := appstatus.Get(err)
	if !ok {
		logging.Errorf(ctx, "Non-appstatus error in a batch response: %s", err)
		return &pb.BatchResponse_Response_Error{Error: grpcStatus.New(codes.Internal, "Internal server error").Proto()}
	}
	return &pb.BatchResponse_Response_Error{Error: st.Proto()}
}
