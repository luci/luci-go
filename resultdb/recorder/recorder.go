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

package main

import (
	"context"

	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// recorderServer implements pb.RecorderServer.
//
// It does not return gRPC-native errors. NewRecorder takes care of that.
type recorderServer struct {
}

// NewRecorderServer creates an implementation of RecorderServer.
func NewRecorderServer() pb.RecorderServer {
	return &pb.DecoratedRecorder{
		Service:  &recorderServer{},
		Postlude: internal.UnwrapGrpcCodePostlude,
	}
}


// FinalizeInvocation implements pb.RecorderServer.
func (s *recorderServer) FinalizeInvocation(ctx context.Context, in *pb.FinalizeInvocationRequest) (*pb.Invocation, error) {
	return nil, grpcutil.Unimplemented
}

// CreateTestResult implements pb.RecorderServer.
func (s *recorderServer) CreateTestResult(ctx context.Context, in *pb.CreateTestResultRequest) (*pb.TestResult, error) {
	return nil, grpcutil.Unimplemented
}

// BatchCreateTestResults implements pb.RecorderServer.
func (s *recorderServer) BatchCreateTestResults(ctx context.Context, in *pb.BatchCreateTestResultsRequest) (*pb.BatchCreateTestResultsResponse, error) {
	return nil, grpcutil.Unimplemented
}

// CreateTestExoneration implements pb.RecorderServer.
func (s *recorderServer) CreateTestExoneration(ctx context.Context, in *pb.CreateTestExonerationRequest) (*pb.TestExoneration, error) {
	return nil, grpcutil.Unimplemented
}

// BatchCreateTestExonerations implements pb.RecorderServer.
func (s *recorderServer) BatchCreateTestExonerations(ctx context.Context, in *pb.BatchCreateTestExonerationsRequest) (*pb.BatchCreateTestExonerationsResponse, error) {
	return nil, grpcutil.Unimplemented
}
