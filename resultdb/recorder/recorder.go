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

	resultspb "go.chromium.org/luci/results/proto/v1"
)

// RecorderServer implements resultspb.RecorderServer.
//
// This is not typically used directly by the end client, but by intermediaries
// such as the Uploader, which handles uploading test results from swarming
// tasks.
type RecorderServer struct {
}

// CreateInvocation implements resultspb.RecorderServer.
func (s *RecorderServer) CreateInvocation(ctx context.Context, in *resultspb.CreateInvocationRequest) (*resultspb.Invocation, error) {
	return nil, grpcutil.Unimplemented
}

// UpdateInvocation implements resultspb.RecorderServer.
func (s *RecorderServer) UpdateInvocation(ctx context.Context, in *resultspb.UpdateInvocationRequest) (*resultspb.Invocation, error) {
	return nil, grpcutil.Unimplemented
}

// FinalizeInvocation implements resultspb.RecorderServer.
func (s *RecorderServer) FinalizeInvocation(ctx context.Context, in *resultspb.FinalizeInvocationRequest) (*resultspb.Invocation, error) {
	return nil, grpcutil.Unimplemented
}

// CreateInclusion implements resultspb.RecorderServer.
func (s *RecorderServer) CreateInclusion(ctx context.Context, in *resultspb.CreateInclusionRequest) (*resultspb.Inclusion, error) {
	return nil, grpcutil.Unimplemented
}

// OverrideInclusion implements resultspb.RecorderServer.
func (s *RecorderServer) OverrideInclusion(ctx context.Context, in *resultspb.OverrideInclusionRequest) (*resultspb.OverrideInclusionResponse, error) {
	return nil, grpcutil.Unimplemented
}

// CreateTestResult implements resultspb.RecorderServer.
func (s *RecorderServer) CreateTestResult(ctx context.Context, in *resultspb.CreateTestResultRequest) (*resultspb.TestResult, error) {
	return nil, grpcutil.Unimplemented
}

// BatchCreateTestResults implements resultspb.RecorderServer.
func (s *RecorderServer) BatchCreateTestResults(ctx context.Context, in *resultspb.BatchCreateTestResultsRequest) (*resultspb.BatchCreateTestResultsResponse, error) {
	return nil, grpcutil.Unimplemented
}

// CreateTestExoneration implements resultspb.RecorderServer.
func (s *RecorderServer) CreateTestExoneration(ctx context.Context, in *resultspb.CreateTestExonerationRequest) (*resultspb.TestExoneration, error) {
	return nil, grpcutil.Unimplemented
}

// BatchCreateTestExonerations implements resultspb.RecorderServer.
func (s *RecorderServer) BatchCreateTestExonerations(ctx context.Context, in *resultspb.BatchCreateTestExonerationsRequest) (*resultspb.BatchCreateTestExonerationsResponse, error) {
	return nil, grpcutil.Unimplemented
}

// DeriveInvocation implements resultspb.RecorderServer.
func (s *RecorderServer) DeriveInvocation(ctx context.Context, in *resultspb.DeriveInvocationRequest) (*resultspb.DeriveInvocationResponse, error) {
	return nil, grpcutil.Unimplemented
}
