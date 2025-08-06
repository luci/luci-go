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
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func emptyOrEqual(name, actual, expected string) error {
	switch actual {
	case "", expected:
		return nil
	}
	return errors.Fmt("%s must be either empty or equal to %q, but %q", name, expected, actual)
}

func validateBatchCreateTestResultsRequest(req *pb.BatchCreateTestResultsRequest, cfg *config.CompiledServiceConfig, now time.Time) error {
	if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
		return errors.Fmt("invocation: %w", err)
	}

	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}

	// TODO: Try to get rid of this case if we can and always expect in.Requests has at least one entry.
	if len(req.Requests) > 0 {
		if err := pbutil.ValidateBatchRequestCountAndSize(req.Requests); err != nil {
			return errors.Fmt("requests: %w", err)
		}
	}

	type Key struct {
		testID   string
		resultID string
	}
	keySet := map[Key]struct{}{}

	for i, r := range req.Requests {
		if err := emptyOrEqual("invocation", r.Invocation, req.Invocation); err != nil {
			return errors.Fmt("requests: %d: %w", i, err)
		}
		if err := emptyOrEqual("request_id", r.RequestId, req.RequestId); err != nil {
			return errors.Fmt("requests: %d: %w", i, err)
		}
		if err := validateTestResult(now, cfg, r.TestResult); err != nil {
			return errors.Fmt("requests: %d: test_result: %w", i, err)
		}

		key := Key{
			testID:   r.TestResult.TestId,
			resultID: r.TestResult.ResultId,
		}
		if _, ok := keySet[key]; ok {
			// Duplicated results.
			return errors.Fmt("duplicate test results in request: testID %q, resultID %q", key.testID, key.resultID)
		}
		keySet[key] = struct{}{}
	}
	return nil
}

// BatchCreateTestResults implements pb.RecorderServer.
func (s *recorderServer) BatchCreateTestResults(ctx context.Context, in *pb.BatchCreateTestResultsRequest) (*pb.BatchCreateTestResultsResponse, error) {
	now := clock.Now(ctx).UTC()
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBatchCreateTestResultsRequest(in, cfg, now); err != nil {
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
		tr, mutation, err := insertTestResult(ctx, invID, in.RequestId, r.TestResult)
		if err != nil {
			return nil, err
		}
		ret.TestResults[i] = tr
		ms[i] = mutation
		if i == 0 {
			commonPrefix = ret.TestResults[i].TestId
		} else {
			commonPrefix = longestCommonPrefix(commonPrefix, ret.TestResults[i].TestId)
		}
		varUnion.AddAll(pbutil.VariantToStrings(ret.TestResults[i].GetVariant()))
	}

	var realm string
	_, err = mutateInvocation(ctx, invID, func(ctx context.Context) error {
		span.BufferWrite(ctx, ms...)
		eg, ctx := errgroup.WithContext(ctx)
		eg.Go(func() (err error) {
			var invCommonTestIdPrefix spanner.NullString
			var invVars []string
			if err = invocations.ReadColumns(ctx, invID, map[string]any{
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
				span.BufferWrite(ctx, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId":           invID,
					"CommonTestIDPrefix":     newPrefix,
					"TestResultVariantUnion": varUnion.ToSortedSlice(),
				}))
			}

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

func insertTestResult(ctx context.Context, invID invocations.ID, requestID string, body *pb.TestResult) (*pb.TestResult, *spanner.Mutation, error) {
	// create a copy of the input message with the OUTPUT_ONLY field(s) to be used in
	// the response
	ret := proto.Clone(body).(*pb.TestResult)

	if ret.TestIdStructured != nil {
		// Use TestVariantIdentifier to set TestId and Variant (OUTPUT_ONLY fields).
		// Also set the OUTPUT_ONLY fields in TestVariantIdentiifer.
		ret.TestId = pbutil.TestIDFromStructuredTestIdentifier(ret.TestIdStructured)
		ret.Variant = pbutil.VariantFromStructuredTestIdentifier(ret.TestIdStructured)
		pbutil.PopulateStructuredTestIdentifierHashes(ret.TestIdStructured)
	} else {
		// Legacy test uploader. Populate TestIdStructured from TestId and Variant
		// so that this RPC returns the same response as ListTestResults.
		var err error
		ret.TestIdStructured, err = pbutil.ParseStructuredTestIdentifierForOutput(ret.TestId, ret.Variant)
		if err != nil {
			// This should not happen, the test identifier should already have been validated.
			return nil, nil, errors.Fmt("parse test identifier: %w", err)
		}
	}
	ret.VariantHash = pbutil.VariantHash(ret.Variant)

	ret.Name = pbutil.LegacyTestResultName(string(invID), ret.TestId, ret.ResultId)

	// handle values for nullable columns
	var runDuration spanner.NullInt64
	if ret.Duration != nil {
		runDuration.Int64 = pbutil.MustDuration(ret.Duration).Microseconds()
		runDuration.Valid = true
	}

	if ret.StatusV2 != pb.TestResult_STATUS_UNSPECIFIED {
		// Populate v1 status from v2 status fields.
		ret.Status, ret.Expected = statusV1FromV2(ret.StatusV2, ret.FailureReason.GetKind(), ret.FrameworkExtensions.GetWebTest())
	} else {
		status, failureKind, webTest := statusV2FromV1(ret.Status, ret.Expected)

		// Populate v2 status fields from v1 status.
		ret.StatusV2 = status
		if status == pb.TestResult_FAILED {
			if ret.FailureReason == nil {
				ret.FailureReason = &pb.FailureReason{}
			}
			ret.FailureReason.Kind = failureKind
		}
		if webTest != nil {
			if ret.FrameworkExtensions == nil {
				ret.FrameworkExtensions = &pb.FrameworkExtensions{}
			}
			ret.FrameworkExtensions.WebTest = webTest
		}
	}

	row := map[string]any{
		"InvocationId":    invID,
		"TestId":          ret.TestId,
		"ResultId":        ret.ResultId,
		"Variant":         ret.Variant,
		"VariantHash":     ret.VariantHash,
		"CommitTimestamp": spanner.CommitTimestamp,
		"IsUnexpected":    spanner.NullBool{Bool: true, Valid: !ret.Expected},
		"Status":          ret.Status,
		"StatusV2":        ret.StatusV2,
		"SummaryHTML":     spanutil.Compressed(ret.SummaryHtml),
		"StartTime":       ret.StartTime,
		"RunDurationUsec": runDuration,
		"Tags":            ret.Tags,
	}
	if ret.SkipReason != pb.SkipReason_SKIP_REASON_UNSPECIFIED {
		// Unspecified is mapped to NULL, so only write if we have some other value.
		row["SkipReason"] = ret.SkipReason
	}
	if ret.TestMetadata != nil {
		row["TestMetadata"] = spanutil.Compressed(pbutil.MustMarshal(ret.TestMetadata))
	}
	if ret.FailureReason != nil {
		// Normalise the failure reason. This handles legacy uploaders which only set
		// PrimaryErrorMessage by pushing it into the Errors collection instead.
		testresults.NormaliseFailureReason(ret.FailureReason)
		row["FailureReason"] = spanutil.Compressed(pbutil.MustMarshal(ret.FailureReason))

		// Populate output only fields after marshalling, as we don't want to store those.
		testresults.PopulateFailureReasonOutputOnlyFields(ret.FailureReason)
	}
	if ret.Properties != nil {
		row["Properties"] = spanutil.Compressed(pbutil.MustMarshal(ret.Properties))
	}
	if ret.SkippedReason != nil {
		row["SkippedReason"] = spanutil.Compressed(pbutil.MustMarshal(ret.SkippedReason))
	}
	if ret.FrameworkExtensions != nil {
		row["FrameworkExtensions"] = spanutil.Compressed(pbutil.MustMarshal(ret.FrameworkExtensions))
	}
	mutation := spanner.InsertOrUpdateMap("TestResults", spanutil.ToSpannerMap(row))
	return ret, mutation, nil
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

// validateTestResult returns a non-nil error if msg is invalid.
func validateTestResult(now time.Time, cfg *config.CompiledServiceConfig, tr *pb.TestResult) error {
	validateToScheme := func(testID pbutil.BaseTestIdentifier) error {
		return validateTestIDToScheme(cfg, testID)
	}
	if err := pbutil.ValidateTestResult(now, validateToScheme, tr); err != nil {
		return err
	}
	return nil
}

func validateTestIDToScheme(cfg *config.CompiledServiceConfig, testID pbutil.BaseTestIdentifier) error {
	scheme, ok := cfg.Schemes[testID.ModuleScheme]
	if !ok {
		return errors.Fmt("module_scheme: scheme %q is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme", testID.ModuleScheme)
	}
	return scheme.Validate(testID)
}

// statusV1FromV2 computes a v1 status (status + expected) from v2 status fields.
// It is used to populate v1 status for clients uploading the v2 status.
func statusV1FromV2(status pb.TestResult_Status, kind pb.FailureReason_Kind, webTest *pb.WebTest) (oldStatus pb.TestStatus, expected bool) {
	if webTest != nil {
		switch webTest.Status {
		case pb.WebTest_PASS:
			oldStatus = pb.TestStatus_PASS
		case pb.WebTest_FAIL:
			oldStatus = pb.TestStatus_FAIL
		case pb.WebTest_TIMEOUT:
			oldStatus = pb.TestStatus_ABORT
		case pb.WebTest_CRASH:
			oldStatus = pb.TestStatus_CRASH
		case pb.WebTest_SKIP:
			oldStatus = pb.TestStatus_SKIP
		default:
			// Web Test Status is closed to extension so this should never happen.
			panic(errors.Fmt("unknown web test status: %v", webTest.Status))
		}
		expected = webTest.IsExpected
		return oldStatus, expected
	}
	switch status {
	case pb.TestResult_PASSED:
		return pb.TestStatus_PASS, true
	case pb.TestResult_FAILED:
		// V1 breaks out different kinds of failures into a different
		// top level status: abort, crash, fail.
		switch kind {
		case pb.FailureReason_TIMEOUT:
			return pb.TestStatus_ABORT, false
		case pb.FailureReason_CRASH:
			return pb.TestStatus_CRASH, false
		default:
			return pb.TestStatus_FAIL, false
		}
	case pb.TestResult_SKIPPED:
		// Intentional skip.
		return pb.TestStatus_SKIP, true
	case pb.TestResult_EXECUTION_ERRORED:
		// Unintentional skip due to some infra error.
		return pb.TestStatus_SKIP, false
	case pb.TestResult_PRECLUDED:
		// Unintentional skip due to some infra error.
		return pb.TestStatus_SKIP, false
	default:
		// Status v2 is closed to extension so this should never happen.
		panic(errors.Fmt("unknown status v2: %v", status))
	}
}

// statusV2FromV1 computes a v2 status from a v1 status.
// It is used to populate v2 status for clients uploading the v1 status.
//
// Between statusV2FromV1 and statusV1FromV2, we require a V1 status to roundtrip
// back to the same V1 status.
func statusV2FromV1(oldStatus pb.TestStatus, expected bool) (status pb.TestResult_Status, kind pb.FailureReason_Kind, webTest *pb.WebTest) {
	if oldStatus == pb.TestStatus_SKIP {
		if expected {
			status = pb.TestResult_SKIPPED
		} else {
			status = pb.TestResult_EXECUTION_ERRORED
		}
		return status, pb.FailureReason_KIND_UNSPECIFIED, nil
	}
	if expected {
		// Expected (non-skipped) result.
		if oldStatus != pb.TestStatus_PASS {
			// Expected failure. This must be from a webtest.
			switch oldStatus {
			case pb.TestStatus_ABORT:
				webTest = &pb.WebTest{
					Status:     pb.WebTest_TIMEOUT,
					IsExpected: true,
				}
			case pb.TestStatus_CRASH:
				webTest = &pb.WebTest{
					Status:     pb.WebTest_CRASH,
					IsExpected: true,
				}
			case pb.TestStatus_FAIL:
				webTest = &pb.WebTest{
					Status:     pb.WebTest_FAIL,
					IsExpected: true,
				}
			default:
				// Status v1 is closed to extension so this should never happen.
				panic(errors.Fmt("unknown status v1: %v", oldStatus))
			}
		}
		return pb.TestResult_PASSED, pb.FailureReason_KIND_UNSPECIFIED, webTest
	}

	// Unexpected (non-skipped) result.
	switch oldStatus {
	case pb.TestStatus_ABORT:
		kind = pb.FailureReason_TIMEOUT
	case pb.TestStatus_CRASH:
		kind = pb.FailureReason_CRASH
	case pb.TestStatus_FAIL:
		kind = pb.FailureReason_ORDINARY
	case pb.TestStatus_PASS:
		kind = pb.FailureReason_ORDINARY
	default:
		// Status v1 is closed to extension so this should never happen.
		panic(errors.Fmt("unknown status v1: %v", oldStatus))
	}

	if oldStatus == pb.TestStatus_PASS {
		// Unexpected pass. This must be from a webtest.
		webTest = &pb.WebTest{
			Status:     pb.WebTest_PASS,
			IsExpected: false,
		}
	}
	return pb.TestResult_FAILED, kind, webTest
}
