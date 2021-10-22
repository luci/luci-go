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
	"go.chromium.org/luci/common/trace"
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

	var goBatchReq []*pb.BatchRequest_Request
	var schBatchReq []*pb.ScheduleBuildRequest
	readReqs := 0
	writeReqs := 0

	// record the mapping of indices in to indices in original req.
	goIndices := make([]int, 0, len(req.Requests))
	schIndices := make([]int, 0, len(req.Requests))
	for i, r := range req.Requests {
		switch r.Request.(type) {
		case *pb.BatchRequest_Request_ScheduleBuild:
			schIndices = append(schIndices, i)
			schBatchReq = append(schBatchReq, r.GetScheduleBuild())
			writeReqs++
		case *pb.BatchRequest_Request_CancelBuild:
			goIndices = append(goIndices, i)
			goBatchReq = append(goBatchReq, r)
			writeReqs++
		case *pb.BatchRequest_Request_GetBuild, *pb.BatchRequest_Request_SearchBuilds:
			goIndices = append(goIndices, i)
			goBatchReq = append(goBatchReq, r)
			readReqs++
		default:
			return nil, appstatus.BadRequest(errors.New("request includes an unsupported type"))
		}
	}

	if readReqs > readReqsSizeLimit {
		return nil, appstatus.BadRequest(errors.Reason("the maximum allowed read request count in Batch is %d.", readReqsSizeLimit).Err())
	}
	if writeReqs > writeReqsSizeLimit {
		return nil, appstatus.BadRequest(errors.Reason("the maximum allowed write request count in Batch is %d.", writeReqsSizeLimit).Err())
	}

	// ID used to log this Batch operation in the pRPC request log (see common.go).
	// Used as the parent request log ID when logging individual operations here.
	parent := trace.SpanContext(ctx)
	err := parallel.WorkPool(64, func(c chan<- func() error) {
		c <- func() (err error) {
			ctx, span := trace.StartSpan(ctx, "Batch.ScheduleBuild")
			// Batch schedule requests. It allows partial success.
			ret, merr := b.scheduleBuilds(ctx, schBatchReq)
			defer span.End(err)
			for i, e := range merr {
				if e != nil {
					res.Responses[schIndices[i]] = &pb.BatchResponse_Response{
						Response: toBatchResponseError(ctx, e),
					}
					logToBQ(ctx, fmt.Sprintf("%s;%d", trace.SpanContext(ctx), schIndices[i]), parent, "ScheduleBuild")
				}
			}
			for i, r := range ret {
				if r != nil {
					res.Responses[schIndices[i]] = &pb.BatchResponse_Response{
						Response: &pb.BatchResponse_Response_ScheduleBuild{
							ScheduleBuild: r,
						},
					}
					logToBQ(ctx, fmt.Sprintf("%s;%d", trace.SpanContext(ctx), schIndices[i]), parent, "ScheduleBuild")
				}
			}
			return nil
		}
		for i, r := range goBatchReq {
			i, r := i, r
			c <- func() (err error) {
				ctx := ctx
				method := ""
				response := &pb.BatchResponse_Response{}
				var span trace.Span
				switch r.Request.(type) {
				case *pb.BatchRequest_Request_GetBuild:
					ctx, span = trace.StartSpan(ctx, "Batch.GetBuild")
					defer span.End(err)
					ret, e := b.GetBuild(ctx, r.GetGetBuild())
					response.Response = &pb.BatchResponse_Response_GetBuild{GetBuild: ret}
					err = e
					method = "GetBuild"
				case *pb.BatchRequest_Request_SearchBuilds:
					ctx, span = trace.StartSpan(ctx, "Batch.SearchBuilds")
					defer span.End(err)
					ret, e := b.SearchBuilds(ctx, r.GetSearchBuilds())
					response.Response = &pb.BatchResponse_Response_SearchBuilds{SearchBuilds: ret}
					err = e
					method = "SearchBuilds"
				case *pb.BatchRequest_Request_CancelBuild:
					ctx, span = trace.StartSpan(ctx, "Batch.CancelBuild")
					defer span.End(err)
					ret, e := b.CancelBuild(ctx, r.GetCancelBuild())
					response.Response = &pb.BatchResponse_Response_CancelBuild{CancelBuild: ret}
					err = e
					method = "CancelBuild"
				default:
					panic(fmt.Sprintf("attempted to handle unexpected request type %T", r.Request))
				}
				logToBQ(ctx, trace.SpanContext(ctx), parent, method)
				if err != nil {
					logging.Warningf(ctx, "Error from Go: %s", err)
					if goErrSt, ok := convertGRPCError(err); ok {
						return appstatus.Error(goErrSt.Code(), goErrSt.Message())
					}
					response.Response = toBatchResponseError(ctx, err)
				}
				res.Responses[goIndices[i]] = response
				span.End(nil)
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
