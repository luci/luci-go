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

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func emptyOrEqual(name, actual, expected string) error {
	switch actual {
	case "", expected:
		return nil
	}
	return errors.Reason("%s must be either empty or equal to %q, but %q", name, expected, actual).Err()
}

func validateBatchCreateTestResultsRequest(req *pb.BatchCreateTestResultsRequest, now time.Time) error {
	if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}

	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}

	if err := pbutil.ValidateBatchRequestCount(len(req.Requests)); err != nil {
		return err
	}

	type Key struct {
		testID   string
		resultID string
	}
	keySet := map[Key]struct{}{}

	for i, r := range req.Requests {
		if err := emptyOrEqual("invocation", r.Invocation, req.Invocation); err != nil {
			return errors.Annotate(err, "requests: %d", i).Err()
		}
		if err := emptyOrEqual("request_id", r.RequestId, req.RequestId); err != nil {
			return errors.Annotate(err, "requests: %d", i).Err()
		}
		if err := pbutil.ValidateTestResult(now, r.TestResult); err != nil {
			return errors.Annotate(err, "requests: %d: test_result", i).Err()
		}

		key := Key{
			testID:   r.TestResult.TestId,
			resultID: r.TestResult.ResultId,
		}
		if _, ok := keySet[key]; ok {
			// Duplicated results.
			return errors.Reason("duplicate test results in request: testID %q, resultID %q", key.testID, key.resultID).Err()
		}
		keySet[key] = struct{}{}
	}
	return nil
}

// BatchCreateTestResults implements pb.RecorderServer.
func (s *recorderServer) BatchCreateTestResults(ctx context.Context, in *pb.BatchCreateTestResultsRequest) (*pb.BatchCreateTestResultsResponse, error) {
	now := clock.Now(ctx).UTC()
	if err := validateBatchCreateTestResultsRequest(in, now); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	invID := invocations.MustParseName(in.Invocation)
	ret := &pb.BatchCreateTestResultsResponse{
		TestResults: make([]*pb.TestResult, len(in.Requests)),
	}
	ms := make([]*spanner.Mutation, len(in.Requests))
	for i, r := range in.Requests {
		ret.TestResults[i], ms[i] = insertTestResult(ctx, invID, in.RequestId, r.TestResult)
	}
	err := mutateInvocation(ctx, invID, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		if err := txn.BufferWrite(ms); err != nil {
			return err
		}
		return invocations.IncrementTestResultCount(ctx, txn, invID, int64(len(in.Requests)))
	})
	if err != nil {
		return nil, err
	}
	span.IncRowCount(ctx, len(in.Requests), span.TestResults, span.Inserted)
	return ret, nil
}
