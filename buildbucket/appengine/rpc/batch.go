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
	"reflect"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// toCamelCase converts snake case to camel case.
func toCamelCase(str string) string {
	return regexp.MustCompile("(^[A-Za-z])|_([A-Za-z])").ReplaceAllStringFunc(
		str,
		func(s string) string {
			return strings.ToUpper(strings.Replace(s, "_", "", -1))
		})
}

// Batch handles a batch request. Implements pb.BuildsServer.
func (*Builds) Batch(ctx context.Context, req *pb.BatchRequest) (*pb.BatchResponse, error) {
	res := &pb.BatchResponse{}
	if req == nil || len(req.Requests) == 0 {
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
		msg := r.ProtoReflect()
		oneof := msg.WhichOneof(msg.Descriptor().Oneofs().ByName("request"))
		switch {
		case oneof == nil:
			return nil, appstatus.BadRequest(errors.Reason("request is not specified").Err())
		case oneof.Name() == "schedule_build" || oneof.Name() == "cancel_build":
			pyIndexMap[len(pyBatchReq.Requests)] = i
			pyBatchReq.Requests = append(pyBatchReq.Requests, r)
		case oneof.Name() == "get_build" || oneof.Name() == "search_builds":
			goIndexMap[len(goBatchReq)] = i
			goBatchReq = append(goBatchReq, r)
		}
	}
	//TODO(yuanjunh): call Py service for schedule and cancel requests.
	buildsService := &Builds{}
	err := parallel.WorkPool(64, func(c chan<- func() error) {
		for i, r := range goBatchReq {
			i, r := i, r
			c <- func() error {
				// dynamically call corresponding method.
				msg := r.ProtoReflect()
				oneof := msg.WhichOneof(msg.Descriptor().Oneofs().ByName("request"))
				reqType := toCamelCase(string(oneof.Name()))
				method := reflect.ValueOf(buildsService).MethodByName(reqType)
				params := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(r).MethodByName("Get" + reqType).Call(nil)[0]}
				ret := method.Call(params)

				// handle the error in response.
				err := ret[1].Interface()
				if err != nil {
					status, ok := appstatus.Get(err.(error))
					if !ok {
						return appstatus.BadRequest(errors.Reason("error is not properly populated").Err())
					}
					res.Responses[goIndexMap[i]] = &pb.BatchResponse_Response{
						Response: &pb.BatchResponse_Response_Error{Error: status.Proto()}}
					return nil
				}

				// handle the normal response.
				response := ret[0].Interface()
				switch reqType {
				case "GetBuild":
					res.Responses[goIndexMap[i]] = &pb.BatchResponse_Response{
						Response: &pb.BatchResponse_Response_GetBuild{GetBuild: response.(*pb.Build)},
					}
				case "SearchBuilds":
					res.Responses[goIndexMap[i]] = &pb.BatchResponse_Response{
						Response: &pb.BatchResponse_Response_SearchBuilds{SearchBuilds: response.(*pb.SearchBuildsResponse)},
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
