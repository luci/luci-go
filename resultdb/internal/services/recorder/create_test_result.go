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

package recorder

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func validateCreateTestResultRequest(msg *pb.CreateTestResultRequest, cfg *config.CompiledServiceConfig, now time.Time) error {
	if err := pbutil.ValidateInvocationName(msg.Invocation); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}
	if err := validateTestResult(now, cfg, msg.TestResult); err != nil {
		return errors.Annotate(err, "test_result").Err()
	}
	if err := pbutil.ValidateRequestID(msg.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}
	return nil
}

// CreateTestResult implements pb.RecorderServer.
func (s *recorderServer) CreateTestResult(ctx context.Context, in *pb.CreateTestResultRequest) (*pb.TestResult, error) {
	now := clock.Now(ctx).UTC()
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateCreateTestResultRequest(in, cfg, now); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Piggy back on BatchCreateTestResults.
	res, err := s.BatchCreateTestResults(ctx, &pb.BatchCreateTestResultsRequest{
		Invocation: in.Invocation,
		Requests:   []*pb.CreateTestResultRequest{in},
		RequestId:  in.RequestId,
	})
	if err != nil {
		return nil, err
	}
	return res.TestResults[0], nil
}
