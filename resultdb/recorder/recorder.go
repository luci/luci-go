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

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// RecorderServer implements pb.RecorderServer.
//
// This is not typically used directly by the end client, but by intermediaries
// such as the Uploader, which handles uploading test results from swarming
// tasks.
type RecorderServer struct {
}

// CreateInvocation implements pb.RecorderServer.
func (s *RecorderServer) CreateInvocation(ctx context.Context, in *pb.CreateInvocationRequest) (*pb.Invocation, error) {
	return nil, grpcutil.Unimplemented
}

// UpdateInvocation implements pb.RecorderServer.
func (s *RecorderServer) UpdateInvocation(ctx context.Context, in *pb.UpdateInvocationRequest) (*pb.Invocation, error) {
	return nil, grpcutil.Unimplemented
}

// FinalizeInvocation implements pb.RecorderServer.
func (s *RecorderServer) FinalizeInvocation(ctx context.Context, in *pb.FinalizeInvocationRequest) (*pb.Invocation, error) {
	return nil, grpcutil.Unimplemented
}

// CreateTestResult implements pb.RecorderServer.
func (s *RecorderServer) CreateTestResult(ctx context.Context, in *pb.CreateTestResultRequest) (*pb.TestResult, error) {
	return nil, grpcutil.Unimplemented
}

// BatchCreateTestResults implements pb.RecorderServer.
func (s *RecorderServer) BatchCreateTestResults(ctx context.Context, in *pb.BatchCreateTestResultsRequest) (*pb.BatchCreateTestResultsResponse, error) {
	return nil, grpcutil.Unimplemented
}

// CreateTestExoneration implements pb.RecorderServer.
func (s *RecorderServer) CreateTestExoneration(ctx context.Context, in *pb.CreateTestExonerationRequest) (*pb.TestExoneration, error) {
	return nil, grpcutil.Unimplemented
}

// BatchCreateTestExonerations implements pb.RecorderServer.
func (s *RecorderServer) BatchCreateTestExonerations(ctx context.Context, in *pb.BatchCreateTestExonerationsRequest) (*pb.BatchCreateTestExonerationsResponse, error) {
	return nil, grpcutil.Unimplemented
}
