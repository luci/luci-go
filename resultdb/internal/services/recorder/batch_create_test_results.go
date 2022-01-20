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
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/uniquetestvariants"
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
	var commonPrefix string
	varUnion := stringset.New(0)
	for i, r := range in.Requests {
		ret.TestResults[i], ms[i] = insertTestResult(ctx, invID, in.RequestId, r.TestResult)
		if i == 0 {
			commonPrefix = r.TestResult.TestId
		} else {
			commonPrefix = longestCommonPrefix(commonPrefix, r.TestResult.TestId)
		}
		varUnion.AddAll(pbutil.VariantToStrings(r.TestResult.GetVariant()))
	}

	var realm string
	var totalUTVCount int
	var utvsToRecord []*uniquetestvariants.UniqueTestVariant
	err := mutateInvocation(ctx, invID, func(ctx context.Context) error {
		span.BufferWrite(ctx, ms...)
		eg, ctx := errgroup.WithContext(ctx)
		eg.Go(func() (err error) {
			var invCommonTestIdPrefix spanner.NullString
			var invVars []string
			if err = invocations.ReadColumns(ctx, invID, map[string]interface{}{
				"Realm":                  &realm,
				"CommonTestIDPrefix":     &invCommonTestIdPrefix,
				"TestResultVariantUnion": &invVars,
			}); err != nil {
				return
			}

			newPrefix := commonPrefix
			if !invCommonTestIdPrefix.IsNull() {
				newPrefix = longestCommonPrefix(invCommonTestIdPrefix.String(), commonPrefix)
			}
			varUnion.AddAll(invVars)

			if invCommonTestIdPrefix.String() != newPrefix || varUnion.Len() > len(invVars) {
				span.BufferWrite(ctx, spanutil.UpdateMap("Invocations", map[string]interface{}{
					"InvocationId":           invID,
					"CommonTestIDPrefix":     newPrefix,
					"TestResultVariantUnion": varUnion.ToSortedSlice(),
				}))
			}

			// Find all the unique test variants in the test results.
			utvMap := uniquetestvariants.FromTestResults(realm, ret.TestResults)
			totalUTVCount = len(utvMap)

			// Find all the recently recorded unique test variants.
			utvIDs := uniquetestvariants.IDsFromMap(utvMap)
			recentlyRecordedUTVIDs, err := uniquetestvariants.FilterIDsRecordedAfter(ctx, utvIDs, time.Now().Add(-24*time.Hour))
			if err != nil {
				return err
			}

			// To reduce the number of writes, we only record unique test variants
			// that were not recorded in the last 24 hours.
			for _, utvID := range recentlyRecordedUTVIDs {
				delete(utvMap, utvID)
			}
			utvsToRecord = uniquetestvariants.FromMap(utvMap)

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

	if len(utvsToRecord) > 0 {
		logging.Infof(ctx, "recording %d/%d unique test variants", len(utvsToRecord), totalUTVCount)
		start := time.Now()

		// Update record time.
		for _, utv := range utvsToRecord {
			utv.LastRecordTime = spanner.CommitTimestamp
		}

		// Record the unique test variants in a separate transaction so this is
		// treated as blind write and won't cause contentions.
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			uniquetestvariants.InsertOrUpdate(ctx, utvsToRecord...)
			return nil
		})

		// Failing to record unique variants is not critical.
		// We don't want to return an error here since the added test results are
		// already committed to spanner.
		if err != nil {
			logging.Errorf(ctx, "failed to record unique test variants")
		}
		logging.Infof(ctx, "recording unique test variants took %s", time.Since(start))
	}

	spanutil.IncRowCount(ctx, len(in.Requests), spanutil.TestResults, spanutil.Inserted, realm)
	return ret, nil
}

func insertTestResult(ctx context.Context, invID invocations.ID, requestID string, body *pb.TestResult) (*pb.TestResult, *spanner.Mutation) {
	// create a copy of the input message with the OUTPUT_ONLY field(s) to be used in
	// the response
	ret := proto.Clone(body).(*pb.TestResult)
	ret.Name = pbutil.TestResultName(string(invID), ret.TestId, ret.ResultId)
	ret.VariantHash = pbutil.VariantHash(ret.Variant)

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
		"VariantHash":     ret.VariantHash,
		"CommitTimestamp": spanner.CommitTimestamp,
		"IsUnexpected":    spanner.NullBool{Bool: true, Valid: !body.Expected},
		"Status":          ret.Status,
		"SummaryHTML":     spanutil.Compressed(ret.SummaryHtml),
		"StartTime":       ret.StartTime,
		"RunDurationUsec": runDuration,
		"Tags":            ret.Tags,
	}
	if ret.TestMetadata != nil {
		if tmd, err := proto.Marshal(ret.TestMetadata); err != nil {
			panic(fmt.Sprintf("failed to marshal TestMetadata to bytes: %q", err))
		} else {
			row["TestMetadata"] = spanutil.Compressed(tmd)
		}
	}
	if ret.FailureReason != nil {
		if fr, err := proto.Marshal(ret.FailureReason); err != nil {
			panic(fmt.Sprintf("failed to marshal FailureReason to bytes: %q", err))
		} else {
			row["FailureReason"] = spanutil.Compressed(fr)
		}
	}
	mutation := spanner.InsertOrUpdateMap("TestResults", spanutil.ToSpannerMap(row))
	return ret, mutation
}

func longestCommonPrefix(str1, str2 string) string {
	for i := 0; i < len(str1) && i < len(str2); i++ {
		if str1[i] != str2[i] {
			return str1[:i]
		}
	}
	if len(str1) <= len(str2) {
		return str1
	}
	return str2
}
