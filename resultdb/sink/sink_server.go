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
package sink

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/resultdb/pbutil"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

// sinkServer implements sinkpb.SinkServer.
type sinkServer struct {
	cfg ServerConfig
}

// ReportTestResults implement sinkpb.SinkServer.
func (s *sinkServer) ReportTestResults(ctx context.Context, in *sinkpb.ReportTestResultsRequest) (*sinkpb.ReportTestResultsResponse, error) {
	now := clock.Now(ctx).UTC()
	for _, tr := range in.TestResults {
		if err := pbutil.ValidateSinkTestResult(now, tr); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad request: %s", err)
		}
	}
	return &sinkpb.ReportTestResultsResponse{}, nil
}
