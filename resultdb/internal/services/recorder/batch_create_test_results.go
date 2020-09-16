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
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	"go.chromium.org/luci/resultdb/internal/spanutil"
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

	var realm string
	err := mutateInvocation(ctx, invID, func(ctx context.Context) error {
		span.BufferWrite(ctx, ms...)
		eg, ctx := errgroup.WithContext(ctx)
		eg.Go(func() (err error) {
			realm, err = invocations.ReadRealm(ctx, invID)
			return
		})
		eg.Go(func() error {
			return resultcount.IncrementTestResultCount(ctx, invID, int64(len(in.Requests)))
		})
		return eg.Wait()
	})
	if err != nil {
		return nil, err
	}
	spanutil.IncRowCount(ctx, len(in.Requests), spanutil.TestResults, spanutil.Inserted, realm)
	return ret, nil
}

func insertTestResult(ctx context.Context, invID invocations.ID, requestID string, body *pb.TestResult) (*pb.TestResult, *spanner.Mutation) {
	// create a copy of the input message with the OUTPUT_ONLY field(s) to be used in
	// the response
	ret := proto.Clone(body).(*pb.TestResult)
	ret.Name = pbutil.TestResultName(string(invID), ret.TestId, ret.ResultId)

	// handle values for nullable columns
	var runDuration spanner.NullInt64
	if ret.Duration != nil {
		runDuration.Int64 = pbutil.MustDuration(ret.Duration).Microseconds()
		runDuration.Valid = true
	}

	row := map[string]interface{}{
		"InvocationId":    invID,
		"TestId":          ret.TestId,
		"ResultId":        ret.ResultId,
		"Variant":         ret.Variant,
		"VariantHash":     pbutil.VariantHash(ret.Variant),
		"CommitTimestamp": spanner.CommitTimestamp,
		"IsUnexpected":    spanner.NullBool{Bool: true, Valid: !body.Expected},
		"Status":          ret.Status,
		"SummaryHTML":     spanutil.Compressed(ret.SummaryHtml),
		"StartTime":       ret.StartTime,
		"RunDurationUsec": runDuration,
		"Tags":            ret.Tags,
	}
	if ret.TestLocation != nil {
		row["TestLocationFileName"] = ret.TestLocation.FileName
		// Spanner client does not support int32
		row["TestLocationLine"] = int(ret.TestLocation.Line)
	}
	mutation := spanner.InsertOrUpdateMap("TestResults", spanutil.ToSpannerMap(row))
	return ret, mutation
}
