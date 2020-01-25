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
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/internal/appstatus"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// recorderServer implements pb.RecorderServer.
//
// It does not return gRPC-native errors. NewRecorder takes care of that.
type recorderServer struct {
	// derivedInvBQTable is the BigQuery table that the derived invocations
	// should be exported to.
	derivedInvBQTable *pb.BigQueryExport
	// Duration since invocation creation after which to delete expected test
	// results.
	expectedResultsExpiration time.Duration
}

// CreateTestResult implements pb.RecorderServer.
func (s *recorderServer) CreateTestResult(ctx context.Context, in *pb.CreateTestResultRequest) (*pb.TestResult, error) {
	return nil, appstatus.Errorf(codes.Unimplemented, "RPC is not implemented yet")
}

// BatchCreateTestResults implements pb.RecorderServer.
func (s *recorderServer) BatchCreateTestResults(ctx context.Context, in *pb.BatchCreateTestResultsRequest) (*pb.BatchCreateTestResultsResponse, error) {
	return nil, appstatus.Errorf(codes.Unimplemented, "RPC is not implemented yet")
}
