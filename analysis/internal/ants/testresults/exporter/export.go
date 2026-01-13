// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package exporter exports test results to BigQuery in AnTS format.
package exporter

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/ants/utils"
	"go.chromium.org/luci/analysis/internal/bqutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.AntsTestResultRow) error
}

// Exporter provides methods to stream test results into BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

// ExportOptions captures context which will be exported
// alongside the test results.
type ExportOptions struct {
	// Exactly one of Invocation or RootInvocation should be set.
	Invocation     *rdbpb.Invocation
	RootInvocation *rdbpb.RootInvocation
}

// Export exports the given test results to BigQuery.
func (e *Exporter) Export(ctx context.Context, testVariants []*rdbpb.TestVariant, opts ExportOptions) error {
	exportRow, err := prepareExportRow(testVariants, opts)
	if err != nil {
		return errors.Fmt("prepare row: %w", err)
	}

	if err := e.client.Insert(ctx, exportRow); err != nil {
		return errors.Fmt("insert rows: %w", err)
	}
	return nil
}

const (
	// PrimaryErrorTypeTagKey is the tag key to record an error type for the primary error
	// (failure_reason.errors[0]).
	PrimaryErrorTypeTagKey = "primary_error_type"

	// PrimaryErrorNameTagKey is the tag key used to record an error identifier
	// (e.g. OUT_OF_QUOTA, MOBLY_TEST_CASE_ERROR, EXPECTED_TESTS_MISMATCH)
	// associated with the primary error (failure_reason.errors[0]).
	PrimaryErrorNameTagKey = "primary_error_name"

	// PrimaryErrorCodeTagKey is a tag key used to record an error code associated with the error
	// name associated with the primary error (failure_reason.errors[0]).
	PrimaryErrorCodeTagKey = "primary_error_code"

	// PrimaryErrorOriginTagKey is the tag key used to record the fully qualified java class name
	// that created and threw the exception associated with the primary error
	// (failure_reason.errors[0]).
	PrimaryErrorOriginTagKey = "primary_error_origin"

	// SkipTriggerTagKey is the tag key used to store the condition that caused the
	// test to be skipped.
	SkipTriggerTagKey = "skip_trigger"

	// SkipBugTagKey is the tag key used to store the buganizer ID for the issue that
	// caused the tests to be skipped.
	SkipBugTagKey = "skip_bug_id"
)

// prepareExportRow prepares a BigQuery export rows.
func prepareExportRow(verdicts []*rdbpb.TestVariant, opts ExportOptions) ([]*bqpb.AntsTestResultRow, error) {
	var invocationIDInDestTable string
	var completionTime *timestamppb.Timestamp

	// Find AnTS invocation ID, and invocation completion time.
	if opts.RootInvocation != nil {
		invocationIDInDestTable = opts.RootInvocation.RootInvocationId
		completionTime = opts.RootInvocation.FinalizeTime
	} else if opts.Invocation != nil {
		var err error
		invocationIDInDestTable, err = rdbpbutil.ParseInvocationName(opts.Invocation.Name)
		if err != nil {
			return nil, errors.Fmt("invalid invocation name %q: %w", opts.Invocation.Name, err)
		}
		completionTime = opts.Invocation.FinalizeTime
	} else {
		return nil, errors.New("logic error: neither Invocation nor RootInvocation is set")
	}

	// Initially allocate enough space for 2 result per test variant,
	// slice will be re-sized if necessary.
	results := make([]*bqpb.AntsTestResultRow, 0, len(verdicts)*2)

	for _, tv := range verdicts {
		// Find AnTS test identifier.
		testIDStructured, err := bqutil.StructuredTestIdentifierRDB(tv.TestId, tv.Variant)
		if err != nil {
			return nil, errors.Fmt("test_id_structured: %w", err)
		}
		moduleParameters := utils.ModuleParametersFromVariants(tv.Variant)
		moduleParametersHash := ""
		if len(moduleParameters) > 0 {
			moduleParametersHash = hashParameters(moduleParameters)
		}
		testIdentifier := &bqpb.AntsTestResultRow_TestIdentifier{
			Module:               testIDStructured.ModuleName,
			ModuleParameters:     moduleParameters,
			ModuleParametersHash: moduleParametersHash,
			TestClass:            fmt.Sprintf("%s.%s", testIDStructured.CoarseName, testIDStructured.FineName),
			ClassName:            testIDStructured.FineName,
			PackageName:          testIDStructured.CoarseName,
			Method:               testIDStructured.CaseName,
		}
		// Find AnTS test identifier hash.
		testIdentifierHash, err := persistentHashTestIdentifier(testIdentifier)
		if err != nil {
			return nil, errors.Fmt("test_identifier_hash: %w", err)
		}

		for _, trb := range tv.Results {
			tr := trb.Result
			// Find AnTS timing.
			timing := &bqpb.Timing{
				CreationTimestamp: tr.StartTime.AsTime().UnixMilli(),
				CompleteTimestamp: tr.StartTime.AsTime().Add(tr.Duration.AsDuration()).UnixMilli(),
				CreationMonth:     tr.StartTime.AsTime().Format("2006-01"),
			}
			// Find AnTS test status.
			testStatus := convertToAnTSStatusV2(tr.StatusV2, tr.FailureReason, tr.SkippedReason)

			// Find AnTS debug Info from ResultDB's failure reason.
			var debugInfo *bqpb.DebugInfo
			if tr.FailureReason != nil {
				errorCode, err := strconv.Atoi(findKeyFromTags(PrimaryErrorCodeTagKey, tr.Tags))
				if err != nil {
					// Don't fail the export if we can't parse the error code.
					errorCode = 0
				}
				var trace string
				if len(tr.FailureReason.Errors) > 0 {
					trace = tr.FailureReason.Errors[0].Trace
				}
				errorType := findKeyFromTags(PrimaryErrorTypeTagKey, tr.Tags)
				debugInfo = &bqpb.DebugInfo{
					ErrorMessage: tr.FailureReason.PrimaryErrorMessage,
					Trace:        trace,
					ErrorType:    bqpb.ErrorType(bqpb.ErrorType_value[errorType]),
					ErrorName:    findKeyFromTags(PrimaryErrorNameTagKey, tr.Tags),
					ErrorCode:    int64(errorCode),
					ErrorOrigin:  findKeyFromTags(PrimaryErrorOriginTagKey, tr.Tags),
				}
			}

			// Find AnTS skip reason or debug_info from ResultDB's skipped reason.
			var skippedReason *bqpb.SkippedReason
			if tr.SkippedReason != nil {
				if testStatus != bqpb.AntsTestResultRow_TEST_SKIPPED {
					// AnTS status is not skipped, ResultDB skipped reason maps to debug info.
					debugInfo = &bqpb.DebugInfo{
						ErrorMessage: tr.SkippedReason.ReasonMessage,
						Trace:        tr.SkippedReason.Trace,
					}
				} else {
					skippedReason = &bqpb.SkippedReason{
						ReasonType:    bqpb.ReasonType_REASON_DEMOTION,
						ReasonMessage: tr.SkippedReason.ReasonMessage,
						Trigger:       findKeyFromTags(SkipTriggerTagKey, tr.Tags),
						BugId:         findKeyFromTags(SkipBugTagKey, tr.Tags),
					}
				}
			}

			// Find AnTS properties.
			properties := utils.ConvertToAnTSStringPair(tr.Tags)

			workUnitID := ""
			if opts.RootInvocation != nil {
				parts, err := rdbpbutil.ParseTestResultName(tr.Name)
				if err != nil {
					return nil, err
				}
				workUnitID = parts.WorkUnitID
			}

			// AnTS attempt_number and run_number are unused, no mapping exists from ResultDB.

			// Aggregation is not included in this export, so flaky_test_cases, aggregation_detail,
			// flaky_modules and parent_test_identifier_id are not populated.

			antsTR := &bqpb.AntsTestResultRow{
				TestResultId:       tr.ResultId,
				InvocationId:       invocationIDInDestTable,
				WorkUnitId:         workUnitID,
				TestIdentifier:     testIdentifier,
				TestStatus:         testStatus,
				TestIdentifierHash: testIdentifierHash,
				// TODO: populate these hashes.
				TestIdentifierId: "",
				TestDefinitionId: "",
				DebugInfo:        debugInfo,
				Timing:           timing,
				Properties:       properties,
				SkippedReason:    skippedReason,
				TestId:           tv.TestId,
				CompletionTime:   completionTime,
			}
			if opts.RootInvocation != nil {
				populateFromRootInvocation(antsTR, opts.RootInvocation)
			}
			results = append(results, antsTR)
		}
	}
	return results, nil
}

func populateFromRootInvocation(antsTR *bqpb.AntsTestResultRow, rootInv *rdbpb.RootInvocation) {
	buildDesc := rootInv.PrimaryBuild.GetAndroidBuild()
	definition := rootInv.Definition

	tr := &bqpb.AntsTestResultRow{
		BuildType:     utils.BuildTypeFromBuildID(buildDesc.BuildId),
		BuildId:       buildDesc.BuildId,
		BuildProvider: "androidbuild",
		Branch:        buildDesc.Branch,
		BuildTarget:   buildDesc.BuildTarget,
		Test: &bqpb.Test{
			Name:       definition.Name,
			Properties: utils.ConvertMapToAnTSStringPair(definition.Properties.Def),
		},
	}
	proto.Merge(antsTR, tr)
}

func convertToAnTSStatusV2(status rdbpb.TestResult_Status, failureReason *rdbpb.FailureReason, skippedReason *rdbpb.SkippedReason) bqpb.AntsTestResultRow_TestStatus {
	switch status {
	case rdbpb.TestResult_PASSED:
		return bqpb.AntsTestResultRow_PASS
	case rdbpb.TestResult_FAILED:
		// ResultDB failed maps to AnTS fail or test_error depends on failureReason.Kind.
		if failureReason != nil {
			switch failureReason.Kind {
			case rdbpb.FailureReason_ORDINARY:
				return bqpb.AntsTestResultRow_FAIL
			case rdbpb.FailureReason_TIMEOUT, rdbpb.FailureReason_CRASH:
				return bqpb.AntsTestResultRow_TEST_ERROR
			}
		}
		return bqpb.AntsTestResultRow_FAIL
	case rdbpb.TestResult_SKIPPED:
		// ResultDB skipped maps to AnTS ignored, assumption_failure or test_skipped depends on skippedReason.Kind.
		if skippedReason != nil {
			switch skippedReason.Kind {
			case rdbpb.SkippedReason_DISABLED_AT_DECLARATION:
				return bqpb.AntsTestResultRow_IGNORED
			case rdbpb.SkippedReason_SKIPPED_BY_TEST_BODY:
				return bqpb.AntsTestResultRow_ASSUMPTION_FAILURE
			case rdbpb.SkippedReason_DEMOTED:
				return bqpb.AntsTestResultRow_TEST_SKIPPED
			}
		}
		return bqpb.AntsTestResultRow_TEST_SKIPPED
	case rdbpb.TestResult_EXECUTION_ERRORED, rdbpb.TestResult_PRECLUDED:
		return bqpb.AntsTestResultRow_TEST_SKIPPED
	default:
		return bqpb.AntsTestResultRow_TEST_STATUS_UNSPECIFIED
	}
}

func findKeyFromTags(key string, tags []*rdbpb.StringPair) string {
	for _, tag := range tags {
		if tag.Key == key {
			return tag.Value
		}
	}
	return ""
}
