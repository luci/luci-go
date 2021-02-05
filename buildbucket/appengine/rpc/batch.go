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
	"net/http"

	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var retryableRPCCodeSet = map[codes.Code]bool{
	codes.DeadlineExceeded:  true,
	codes.ResourceExhausted: true,
	codes.Unavailable:       true,
	codes.Unknown:           true,
}

// pyBatchResponse captures the BatchResponse from Py service.
type pyBatchResponse struct {
	res *pb.BatchResponse
	err error
}

// Batch handles a batch request. Implements pb.BuildsServer.
func (b *Builds) Batch(ctx context.Context, req *pb.BatchRequest) (*pb.BatchResponse, error) {
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
		case *pb.BatchRequest_Request_ScheduleBuild, *pb.BatchRequest_Request_CancelBuild:
			pyIndices = append(pyIndices, i)
			pyBatchReq.Requests = append(pyBatchReq.Requests, r)
		case *pb.BatchRequest_Request_GetBuild, *pb.BatchRequest_Request_SearchBuilds:
			goIndices = append(goIndices, i)
			goBatchReq = append(goBatchReq, r)
		default:
			return nil, appstatus.BadRequest(errors.New("request includes an unsupported type"))
		}
	}
	// TODO(crbug.com/1144958): remove calling py after ScheduleBuild and CancelBuild are done.
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
				default:
					panic("impossible")
				}
				if err != nil {
					logging.Warningf(ctx, "Error from Go: %s", err)
					if ok, goErrSt := isBatchRetryableError(err); ok {
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
		ok, pyErrSt := isBatchRetryableError(pyRes.err)
		if ok {
			return nil, appstatus.Error(pyErrSt.Code(), pyErrSt.Message())
		}
		for _, idx := range pyIndices {
			res.Responses[idx] = &pb.BatchResponse_Response{
				Response: &pb.BatchResponse_Response_Error{Error: grpcStatus.New(pyErrSt.Code(), pyErrSt.Message()).Proto()},
			}
		}
	} else {
		for i, idx := range pyIndices {
			res.Responses[idx] = pyRes.res.Responses[i]
		}
	}

	return res, nil
}

// isBatchRetryableError returns true with grpc status, if this error has the
// grpc retryable code. Otherwise it returns false with a status which has
// codes.Unknown and the original error message.
func isBatchRetryableError(err error) (bool, *grpcStatus.Status) {
	gStatus, ok := grpcStatus.FromError(err)
	if !ok {
		return false, gStatus
	}
	switch {
	case grpcutil.IsTransientCode(gStatus.Code()):
		return true, gStatus
	case gStatus.Code() == codes.DeadlineExceeded:
		return true, grpcStatus.New(codes.Internal, gStatus.Message())
	}
	return false, gStatus
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
	if b.testPyBuildsClient != nil {
		return b.testPyBuildsClient, nil
	}

	pyHost := "default-dot-cr-buildbucket.appspot.com"
	if ctx.Value("env") == "Dev" {
		pyHost = "default-dot-cr-buildbucket-dev.appspot.com"
	}
	t, err := auth.GetRPCTransport(ctx, auth.AsCredentialsForwarder)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get RPC transport to python BB service").Err()
	}
	pClient := &prpc.Client{
		C:          &http.Client{Transport: t},
		Host:       pyHost,
		PathPrefix: "/python/prpc",
	}
	return pb.NewBuildsPRPCClient(pClient), nil
}
