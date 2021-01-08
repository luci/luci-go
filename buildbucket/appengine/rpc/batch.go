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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// Batch handles a batch request. Implements pb.BuildsServer.
func (b *Builds) Batch(ctx context.Context, req *pb.BatchRequest) (*pb.BatchResponse, error) {
	res := &pb.BatchResponse{}
	if len(req.GetRequests()) == 0 {
		return res, nil
	}
	res.Responses = make([]*pb.BatchResponse_Response, len(req.Requests))

	// schedule and cancel requests are sent to Py service for now.
	pyBatchReq := &pb.BatchRequest{
		Requests: []*pb.BatchRequest_Request{},
	}
	var goBatchReq []*pb.BatchRequest_Request

	// indices in py/goBatchReq to indices in original req
	pyIndexMap := make(map[int]int)
	goIndexMap := make(map[int]int)
	for i, r := range req.Requests {
		switch r.Request.(type) {
		case *pb.BatchRequest_Request_ScheduleBuild, *pb.BatchRequest_Request_CancelBuild:
			pyIndexMap[len(pyBatchReq.Requests)] = i
			pyBatchReq.Requests = append(pyBatchReq.Requests, r)
		case *pb.BatchRequest_Request_GetBuild, *pb.BatchRequest_Request_SearchBuilds:
			goIndexMap[len(goBatchReq)] = i
			goBatchReq = append(goBatchReq, r)
		default:
			return nil, appstatus.BadRequest(errors.New("request includes an unsupported type"))
		}
	}
	// TODO(yuanjunh): call Py service for schedule and cancel requests.
	err := parallel.WorkPool(64, func(c chan<- func() error) {
		for i, r := range goBatchReq {
			i, r := i, r
			c <- func() error {
				res.Responses[goIndexMap[i]] = &pb.BatchResponse_Response{}
				switch r.Request.(type) {
				case *pb.BatchRequest_Request_GetBuild:
					ret, e := b.GetBuild(ctx, r.GetGetBuild())
					if e != nil {
						res.Responses[goIndexMap[i]].Response = toBatchResponseError(e)
					} else {
						res.Responses[goIndexMap[i]].Response = &pb.BatchResponse_Response_GetBuild{GetBuild: ret}
					}
				case *pb.BatchRequest_Request_SearchBuilds:
					ret, e := b.SearchBuilds(ctx, r.GetSearchBuilds())
					if e != nil {
						res.Responses[goIndexMap[i]].Response = toBatchResponseError(e)
					} else {
						res.Responses[goIndexMap[i]].Response = &pb.BatchResponse_Response_SearchBuilds{SearchBuilds: ret}
					}
				}
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// toBatchResponseError converts an error to BatchResponse_Response_Error type.
func toBatchResponseError(err error) *pb.BatchResponse_Response_Error {
	st, ok := appstatus.Get(err.(error))
	if !ok {
		return &pb.BatchResponse_Response_Error{Error: status.New(codes.Internal, err.Error()).Proto()}
	}
	return &pb.BatchResponse_Response_Error{Error: st.Proto()}
}
