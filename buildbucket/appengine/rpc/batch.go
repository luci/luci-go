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
	"net/http"

	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	readReqsSizeLimit  = 1000
	writeReqsSizeLimit = 200
)

var testFakeTransportError = "used in tests only to mock the transport error"

// pyBatchResponse captures the BatchResponse from Py service.
type pyBatchResponse struct {
	res *pb.BatchResponse
	err error
}

// pctKey is the key to a traffic split percentage in [0, 100] in the context.
var pctKey = "pct"

// WithTrafficSplit returns a new context with the given traffic split.
func WithTrafficSplit(c context.Context, pct int) context.Context {
	return context.WithValue(c, &pctKey, pct)
}

// getTrafficSplit returns the traffic split percentage in the given context.
func getTrafficSplit(c context.Context) int {
	pct := c.Value(&pctKey)
	if pct == nil {
		return 0
	}
	return pct.(int)
}

// Batch handles a batch request. Implements pb.BuildsServer.
func (b *Builds) Batch(ctx context.Context, req *pb.BatchRequest) (*pb.BatchResponse, error) {
	pct := getTrafficSplit(ctx)
	res := &pb.BatchResponse{}
	if len(req.GetRequests()) == 0 {
		return res, nil
	}
	res.Responses = make([]*pb.BatchResponse_Response, len(req.Requests))

	// schedule and cancel requests are sent to Py service for now.
	pyBatchReq := &pb.BatchRequest{}
	var goBatchReq []*pb.BatchRequest_Request

	// record the mapping of indices in py/goBatchReq to indices in original req.
	pyIndices := make([]int, 0, len(req.Requests))
	goIndices := make([]int, 0, len(req.Requests))
	for i, r := range req.Requests {
		switch r.Request.(type) {
		case *pb.BatchRequest_Request_ScheduleBuild:
			if mathrand.Intn(ctx, 100) < pct {
				logDetails(ctx, "Batch (ScheduleBuild)", r)
				goIndices = append(goIndices, i)
				goBatchReq = append(goBatchReq, r)
			} else {
				pyIndices = append(pyIndices, i)
				pyBatchReq.Requests = append(pyBatchReq.Requests, r)
			}
		case *pb.BatchRequest_Request_GetBuild, *pb.BatchRequest_Request_SearchBuilds, *pb.BatchRequest_Request_CancelBuild:
			goIndices = append(goIndices, i)
			goBatchReq = append(goBatchReq, r)
		default:
			return nil, appstatus.BadRequest(errors.New("request includes an unsupported type"))
		}
	}

	// validate request count
	// TODO(crbug.com/1144958): refactor code to correctly calculate read and write req count after ScheduleBuild is done.
	// pyBatchReq current only contains ScheduleBuild requests.
	if len(pyBatchReq.Requests) > writeReqsSizeLimit {
		return nil, appstatus.BadRequest(errors.Reason("the maximum allowed write request count in Batch is %d.", writeReqsSizeLimit).Err())
	}
	if len(goBatchReq) > readReqsSizeLimit {
		return nil, appstatus.BadRequest(errors.Reason("the maximum allowed read request count in Batch is %d.", readReqsSizeLimit).Err())
	}

	// TODO(crbug.com/1144958): remove calling py after ScheduleBuild is done.
	pyResC := make(chan *pyBatchResponse)
	if len(pyBatchReq.Requests) != 0 {
		go func() {
			pyClient, err := b.newPyBBClient(ctx)
			if err != nil {
				pyResC <- &pyBatchResponse{res: nil, err: err}
				return
			}
			logging.Debugf(ctx, "Batch: calling python service")
			res, err := pyClient.Batch(ctx, pyBatchReq)
			pyResC <- &pyBatchResponse{res: res, err: err}
		}()
	}

	err := parallel.WorkPool(64, func(c chan<- func() error) {
		for i, r := range goBatchReq {
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
				res.Responses[goIndices[i]] = response
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}

	if len(pyBatchReq.Requests) == 0 {
		return res, nil
	}

	pyRes := <-pyResC
	if pyRes.err != nil {
		logging.Warningf(ctx, "Error from Python service: %s", pyRes.err)
		gStatus, _ := convertGRPCError(pyRes.err)
		return nil, appstatus.Error(gStatus.Code(), gStatus.Message())
	}

	for i, idx := range pyIndices {
		res.Responses[idx] = pyRes.res.Responses[i]
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

// newPyBBClient constructs a BuildBucket python client.
func (b *Builds) newPyBBClient(ctx context.Context) (pb.BuildsClient, error) {
	switch fakeErr, ok := ctx.Value(&testFakeTransportError).(error); {
	case ok:
		return nil, fakeErr
	case b.testPyBuildsClient != nil:
		return b.testPyBuildsClient, nil
	}

	pyHost := "default-dot-cr-buildbucket.appspot.com"
	if ctx.Value("env") == "Dev" {
		pyHost = "default-dot-cr-buildbucket-dev.appspot.com"
	}
	t, err := auth.GetRPCTransport(ctx, auth.AsCredentialsForwarder)
	if err != nil {
		logging.Errorf(ctx, "failed to get Py BB RPC transport: %s", err)
		return nil, grpcStatus.Error(codes.Internal, "failed to get Py BB RPC transport")
	}
	pClient := &prpc.Client{
		C:          &http.Client{Transport: t},
		Host:       pyHost,
		PathPrefix: "/python/prpc",
		Options: &prpc.Options{
			Retry: retry.None,
		},
	}
	return pb.NewBuildsPRPCClient(pClient), nil
}
