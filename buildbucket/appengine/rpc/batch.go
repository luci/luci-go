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

	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

var tracer = otel.Tracer("go.chromium.org/luci/buildbucket")

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

	// Record the mapping of indices in to indices in original req.
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
		case *pb.BatchRequest_Request_GetBuild, *pb.BatchRequest_Request_SearchBuilds, *pb.BatchRequest_Request_GetBuildStatus:
			goIndices = append(goIndices, i)
			goBatchReq = append(goBatchReq, r)
			readReqs++
		default:
			return nil, appstatus.BadRequest(errors.New("request includes an unsupported type"))
		}
	}

	if readReqs > readReqsSizeLimit {
		return nil, appstatus.BadRequest(errors.Fmt("the maximum allowed read request count in Batch is %d.", readReqsSizeLimit))
	}
	if writeReqs > writeReqsSizeLimit {
		return nil, appstatus.BadRequest(errors.Fmt("the maximum allowed write request count in Batch is %d.", writeReqsSizeLimit))
	}

	// ID used to log this Batch operation in the pRPC request log (see common.go).
	// Used as the parent request log ID when logging individual operations here.
	parent := trace.SpanContextFromContext(ctx).TraceID().String()
	err := parallel.WorkPool(64, func(c chan<- func() error) {
		if len(schBatchReq) > 0 {
			c <- func() (err error) {
				ctx, span := tracer.Start(ctx, "Batch.ScheduleBuild")
				// Batch schedule requests. It allows partial success.
				ret, merr := b.scheduleBuilds(ctx, schBatchReq)
				defer func() { endSpan(span, err) }()
				for i := range schBatchReq {
					if reqErr := merr[i]; reqErr != nil {
						res.Responses[schIndices[i]] = &pb.BatchResponse_Response{
							Response: toBatchResponseError(ctx, reqErr),
						}
					} else {
						res.Responses[schIndices[i]] = &pb.BatchResponse_Response{
							Response: &pb.BatchResponse_Response_ScheduleBuild{
								ScheduleBuild: ret[i],
							},
						}
					}
					logToBQ(ctx, fmt.Sprintf("%s;%d", parent, schIndices[i]), parent, "ScheduleBuild")
				}
				return nil
			}
		}
		for i, r := range goBatchReq {
			c <- func() (err error) {
				ctx := ctx
				method := ""
				response := &pb.BatchResponse_Response{}

				var span trace.Span // opened below
				defer func() { endSpan(span, err) }()

				switch r.Request.(type) {
				case *pb.BatchRequest_Request_GetBuild:
					ctx, span = tracer.Start(ctx, "Batch.GetBuild")
					ret, e := b.GetBuild(ctx, r.GetGetBuild())
					response.Response = &pb.BatchResponse_Response_GetBuild{GetBuild: ret}
					err = e
					method = "GetBuild"
				case *pb.BatchRequest_Request_SearchBuilds:
					ctx, span = tracer.Start(ctx, "Batch.SearchBuilds")
					ret, e := b.SearchBuilds(ctx, r.GetSearchBuilds())
					response.Response = &pb.BatchResponse_Response_SearchBuilds{SearchBuilds: ret}
					err = e
					method = "SearchBuilds"
				case *pb.BatchRequest_Request_CancelBuild:
					ctx, span = tracer.Start(ctx, "Batch.CancelBuild")
					ret, e := b.CancelBuild(ctx, r.GetCancelBuild())
					response.Response = &pb.BatchResponse_Response_CancelBuild{CancelBuild: ret}
					err = e
					method = "CancelBuild"
				case *pb.BatchRequest_Request_GetBuildStatus:
					ctx, span = tracer.Start(ctx, "Batch.GetBuildStatus")
					ret, e := b.GetBuildStatus(ctx, r.GetGetBuildStatus())
					response.Response = &pb.BatchResponse_Response_GetBuildStatus{GetBuildStatus: ret}
					err = e
					method = "GetBuildStatus"
				default:
					panic(fmt.Sprintf("attempted to handle unexpected request type %T", r.Request))
				}
				logToBQ(ctx, fmt.Sprintf("%s;%d", parent, goIndices[i]), parent, method)
				if err != nil {
					response.Response = toBatchResponseError(ctx, err)
				}
				res.Responses[goIndices[i]] = response
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// endSpan closes a tracing span.
func endSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	span.End()
}

// toBatchResponseError converts an error to BatchResponse_Response_Error type.
func toBatchResponseError(ctx context.Context, err error) *pb.BatchResponse_Response_Error {
	if errors.Contains(err, context.DeadlineExceeded) {
		return &pb.BatchResponse_Response_Error{Error: grpcStatus.New(codes.DeadlineExceeded, "deadline exceeded").Proto()}
	}
	st, ok := appstatus.Get(err)
	if !ok {
		if gStatus, ok := grpcStatus.FromError(err); ok {
			return &pb.BatchResponse_Response_Error{Error: gStatus.Proto()}
		}
		logging.Errorf(ctx, "Non-appstatus and non-grpc error in a batch response: %s", err)
		return &pb.BatchResponse_Response_Error{Error: grpcStatus.New(codes.Internal, "Internal server error").Proto()}
	}
	return &pb.BatchResponse_Response_Error{Error: st.Proto()}
}
