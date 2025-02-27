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
	"go.chromium.org/luci/common/validate"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
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

func validateBatchCreateTestResultsRequest(req *pb.BatchCreateTestResultsRequest, cfg *config.CompiledServiceConfig, now time.Time) error {
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
		if err := ValidateTestResult(now, cfg, r.TestResult); err != nil {
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
		ret.TestResults[i], ms[i] = insertTestResult(ctx, invID, in.RequestId, r.TestResult)
		if i == 0 {
			commonPrefix = ret.TestResults[i].TestId
		} else {
			commonPrefix = longestCommonPrefix(commonPrefix, ret.TestResults[i].TestId)
		}
		varUnion.AddAll(pbutil.VariantToStrings(ret.TestResults[i].GetVariant()))
	}

	var realm string
	err = mutateInvocation(ctx, invID, func(ctx context.Context) error {
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

func insertTestResult(ctx context.Context, invID invocations.ID, requestID string, body *pb.TestResult) (*pb.TestResult, *spanner.Mutation) {
	// create a copy of the input message with the OUTPUT_ONLY field(s) to be used in
	// the response
	ret := proto.Clone(body).(*pb.TestResult)

	if ret.TestVariantIdentifier != nil {
		// Use TestVariantIdentifier to set TestId and Variant (OUTPUT_ONLY fields).
		// Also set the OUTPUT_ONLY fields in TestVariantIdentiifer.
		ret.TestId = pbutil.TestIDFromTestVariantIdentifier(ret.TestVariantIdentifier)
		ret.Variant = pbutil.VariantFromTestVariantIdentifier(ret.TestVariantIdentifier)
		pbutil.PopulateTestVariantIdentifierHashes(ret.TestVariantIdentifier)
	} else {
		// Legacy test uploader.
		// While we could use TestId and Variant to set TestVariantIdentifier,
		// we do not return this structure to legacy clients to avoid changing
		// API behaviour.
	}
	ret.VariantHash = pbutil.VariantHash(ret.Variant)

	ret.Name = pbutil.TestResultName(string(invID), ret.TestId, ret.ResultId)

	// handle values for nullable columns
	var runDuration spanner.NullInt64
	if ret.Duration != nil {
		runDuration.Int64 = pbutil.MustDuration(ret.Duration).Microseconds()
		runDuration.Valid = true
	}

	row := map[string]any{
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
	if ret.SkipReason != pb.SkipReason_SKIP_REASON_UNSPECIFIED {
		// Unspecified is mapped to NULL, so only write if we have some other value.
		row["SkipReason"] = ret.SkipReason
	}
	if ret.TestMetadata != nil {
		row["TestMetadata"] = spanutil.Compressed(pbutil.MustMarshal(ret.TestMetadata))
	}
	if ret.FailureReason != nil {
		row["FailureReason"] = spanutil.Compressed(pbutil.MustMarshal(ret.FailureReason))
	}
	if ret.Properties != nil {
		row["Properties"] = spanutil.Compressed(pbutil.MustMarshal(ret.Properties))
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

// ValidateTestResult returns a non-nil error if msg is invalid.
func ValidateTestResult(now time.Time, cfg *config.CompiledServiceConfig, tr *pb.TestResult) error {
	if tr == nil {
		return validate.Unspecified()
	}
	if tr.TestVariantIdentifier == nil && tr.TestId != "" {
		// For backwards compatibility, we still accept legacy uploaders setting
		// the test_id and variant fields (even though they are officially OUTPUT_ONLY now).
		testID, err := pbutil.ParseAndValidateTestID(tr.TestId)
		if err != nil {
			return errors.Annotate(err, "test_id").Err()
		}
		if err := pbutil.ValidateVariant(tr.Variant); err != nil {
			return errors.Annotate(err, "variant").Err()
		}
		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := ValidateTestIDToScheme(cfg, testID); err != nil {
			return errors.Annotate(err, "test_id").Err()
		}
	} else {
		// Not a legacy uploader.
		// The TestId and Variant fields are treated as output only as per
		// the API spec and should be ignored. Instead read from the TestVariantIdentifier field.

		if err := pbutil.ValidateTestVariantIdentifier(tr.TestVariantIdentifier); err != nil {
			return errors.Annotate(err, "test_variant_identifier").Err()
		}
		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := ValidateTestIDToScheme(cfg, pbutil.ExtractTestIdentifier(tr.TestVariantIdentifier)); err != nil {
			return errors.Annotate(err, "test_variant_identifier").Err()
		}
	}

	if err := pbutil.ValidateResultID(tr.ResultId); err != nil {
		return errors.Annotate(err, "result_id").Err()
	}
	if err := pbutil.ValidateTestResultStatus(tr.Status); err != nil {
		return errors.Annotate(err, "status").Err()
	}
	if err := pbutil.ValidateSummaryHTML(tr.SummaryHtml); err != nil {
		return errors.Annotate(err, "summary_html").Err()
	}
	if err := pbutil.ValidateStartTimeWithDuration(now, tr.StartTime, tr.Duration); err != nil {
		return err
	}
	if err := pbutil.ValidateStringPairs(tr.Tags); err != nil {
		return errors.Annotate(err, "tags").Err()
	}
	if tr.TestMetadata != nil {
		if err := pbutil.ValidateTestMetadata(tr.TestMetadata); err != nil {
			return errors.Annotate(err, "test_metadata").Err()
		}
	}
	if tr.FailureReason != nil {
		if err := pbutil.ValidateFailureReason(tr.FailureReason); err != nil {
			return errors.Annotate(err, "failure_reason").Err()
		}
	}
	if tr.Properties != nil {
		if err := pbutil.ValidateTestResultProperties(tr.Properties); err != nil {
			return errors.Annotate(err, "properties").Err()
		}
	}
	if err := pbutil.ValidateTestResultSkipReason(tr.Status, tr.SkipReason); err != nil {
		return errors.Annotate(err, "skip_reason").Err()
	}
	return nil
}

func ValidateTestIDToScheme(cfg *config.CompiledServiceConfig, testID pbutil.TestIdentifier) error {
	scheme, ok := cfg.Schemes[testID.ModuleScheme]
	if !ok {
		return errors.Reason("module_scheme: scheme %q is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme", testID.ModuleScheme).Err()
	}
	if err := ValidateTestIDComponent(testID.CoarseName, testID.ModuleScheme, scheme.Coarse); err != nil {
		return errors.Annotate(err, "coarse_name").Err()
	}
	if err := ValidateTestIDComponent(testID.FineName, testID.ModuleScheme, scheme.Fine); err != nil {
		return errors.Annotate(err, "fine_name").Err()
	}
	if err := ValidateTestIDComponent(testID.CaseName, testID.ModuleScheme, scheme.Case); err != nil {
		return errors.Annotate(err, "case_name").Err()
	}
	return nil
}

func ValidateTestIDComponent(component, scheme string, level *config.SchemeLevel) error {
	if level != nil {
		if component == "" {
			return errors.Reason("required, please set a %s (scheme %q)", level.HumanReadableName, scheme).Err()
		}
		if level.ValidationRegexp != nil && !level.ValidationRegexp.MatchString(component) {
			return errors.Reason("does not match validation regexp %q, please set a valid %s (scheme %q)", level.ValidationRegexp.String(), level.HumanReadableName, scheme).Err()
		}
	} else {
		if component != "" {
			return errors.Reason("expected empty value (level is not defined by scheme %q)", scheme).Err()
		}
	}
	return nil
}
