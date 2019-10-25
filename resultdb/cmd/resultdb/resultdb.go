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
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// resultDBServer implements pb.ResultDBServer.
//
// It does not return gRPC-native errors. NewResultDBServer takes care of that.
type resultDBServer struct {
}

// NewResultDBServer creates an implementation of resultDBServer.
func NewResultDBServer() pb.ResultDBServer {
	return &pb.DecoratedResultDB{
		Service:  &resultDBServer{},
		Postlude: internal.UnwrapGrpcCodePostlude,
	}
}

func (s *resultDBServer) GetTestResult(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error) {
	return nil, grpcutil.Unimplemented
}

func (s *resultDBServer) ListTestResults(ctx context.Context, in *pb.ListTestResultsRequest) (*pb.ListTestResultsResponse, error) {
	return nil, grpcutil.Unimplemented
}

func (s *resultDBServer) GetTestExoneration(ctx context.Context, in *pb.GetTestExonerationRequest) (*pb.TestExoneration, error) {
	return nil, grpcutil.Unimplemented
}

func (s *resultDBServer) ListTestExonerations(ctx context.Context, in *pb.ListTestExonerationsRequest) (*pb.ListTestExonerationsResponse, error) {
	return nil, grpcutil.Unimplemented
}

func (s *resultDBServer) QueryTestResults(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
	return nil, grpcutil.Unimplemented
}
