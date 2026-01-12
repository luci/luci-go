// Copyright 2026 The LUCI Authors.
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

package testverdictsv2

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// CreateTestData creates test data used for testing test verdicts.
func CreateTestData(rootInvID rootinvocations.ID) []*spanner.Mutation {
	// Pick a few shards. In reality each test may be in a different shard.
	shard := rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 0}

	baseBuilder := func() *testresultsv2.Builder {
		return testresultsv2.NewBuilder().WithRootInvocationShardID(shard).
			WithModuleName("m1").WithModuleScheme("junit").WithModuleVariant(pbutil.Variant("key", "value")).
			WithCoarseName("c1").WithFineName("f1").WithCaseName("t1").WithTags(pbutil.StringPairs("mytag", "myvalue")).
			WithTestMetadata(&pb.TestMetadata{Name: "tmd", Location: &pb.TestLocation{Repo: "https://repo", FileName: "file"}}).
			WithProperties(&structpb.Struct{Fields: map[string]*structpb.Value{"key": structpb.NewStringValue("value")}})
	}
	baseExonerationBuilder := func() *testexonerationsv2.Builder {
		return testexonerationsv2.NewBuilder().WithRootInvocationShardID(shard).
			WithModuleName("m1").WithModuleScheme("junit").WithModuleVariant(pbutil.Variant("key", "value")).WithCoarseName("c1").WithFineName("f1").WithCaseName("t1")
	}
	workUnitBuilder := func(workUnitID string) *workunits.Builder {
		id := &pb.ModuleIdentifier{
			ModuleName:    "m1",
			ModuleScheme:  "junit",
			ModuleVariant: pbutil.Variant("key", "value"),
		}
		return workunits.NewBuilder(rootInvID, workUnitID).WithModuleID(id)
	}

	// Typically, each result is not in its own realm as it inherits its realm from the
	// work unit, but for testing purposes we put each in its own realm as it makes life easier.
	results := []*testresultsv2.TestResultRow{
		// Verdict 1: Passed
		baseBuilder().WithCaseName("t1").WithResultID("r1").WithStatusV2(pb.TestResult_PASSED).WithRealm("testproject:t1-r1").Build(),

		// Verdict 2: Failed
		baseBuilder().WithCaseName("t2").WithResultID("r1").WithStatusV2(pb.TestResult_FAILED).WithRealm("testproject:t2-r1").
			WithFailureReason(longFailureReason()).Build(),

		// Verdict 3: Flaky (Pass + Fail)
		baseBuilder().WithCaseName("t3").WithResultID("r1").WithStatusV2(pb.TestResult_FAILED).WithRealm("testproject:t3-r1").Build(),
		baseBuilder().WithCaseName("t3").WithResultID("r2").WithStatusV2(pb.TestResult_PASSED).WithRealm("testproject:t3-r2").Build(),

		// Verdict 4: Skipped
		baseBuilder().WithCaseName("t4").WithResultID("r1").WithStatusV2(pb.TestResult_SKIPPED).WithRealm("testproject:t4-r1").
			WithSkippedReason(longSkippedReason()).Build(),

		// Verdict 5: Exonerated (Fail + Exoneration)
		baseBuilder().WithCaseName("t5").WithFineName("f2").WithResultID("r1").WithStatusV2(pb.TestResult_FAILED).WithRealm("testproject:t5-r1").Build(),

		// Verdict 6: Precluded
		baseBuilder().WithCaseName("t6").WithCoarseName("c2").WithResultID("r1").WithStatusV2(pb.TestResult_PRECLUDED).WithRealm("testproject:t6-r1").Build(),

		// Verdict 7: Execution Errored
		baseBuilder().WithCaseName("t7").WithModuleName("m2").WithResultID("r1").WithStatusV2(pb.TestResult_EXECUTION_ERRORED).WithRealm("testproject:t7-r1").Build(),
	}

	exonerations := []*testexonerationsv2.TestExonerationRow{
		// Exonerate t5 (twice)
		baseExonerationBuilder().WithFineName("f2").WithCaseName("t5").WithExonerationID("e1").WithReason(pb.ExonerationReason_OCCURS_ON_OTHER_CLS).Build(),
		baseExonerationBuilder().WithFineName("f2").WithCaseName("t5").WithExonerationID("e2").WithReason(pb.ExonerationReason_NOT_CRITICAL).Build(),
	}

	// Define work units with realms corresponding to the test results. QueryTestVerdicts RPC
	// relies upon this to identify all realms used in the root invocation.
	wus := []*workunits.WorkUnitRow{
		workUnitBuilder("t1-r1-wu").WithRealm("testproject:t1-r1").Build(),
		workUnitBuilder("t2-r1-wu").WithRealm("testproject:t2-r1").Build(),
		workUnitBuilder("t3-r1-wu").WithRealm("testproject:t3-r1").Build(),
		workUnitBuilder("t3-r2-wu").WithRealm("testproject:t3-r2").Build(),
		workUnitBuilder("t4-r1-wu").WithRealm("testproject:t4-r1").Build(),
		workUnitBuilder("t5-r1-wu").WithRealm("testproject:t5-r1").Build(),
		workUnitBuilder("t6-r1-wu").WithRealm("testproject:t6-r1").Build(),
		workUnitBuilder("t7-r1-wu").WithRealm("testproject:t7-r1").Build(),
	}

	// Prepare mutations.
	var ms []*spanner.Mutation
	for _, r := range results {
		ms = append(ms, testresultsv2.InsertForTesting(r))
	}
	for _, e := range exonerations {
		ms = append(ms, testexonerationsv2.InsertForTesting(e))
	}
	for _, wu := range wus {
		ms = append(ms, workunits.InsertForTesting(wu)...)
	}
	return ms
}

// flatTestID returns the test ID of the given case name. It matches
// CreateTestData logic.
func flatTestID(moduleName, coarseName, fineName, caseName string) string {
	return pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(testID(moduleName, coarseName, fineName, caseName)))
}

// testID returns a test identifier used for testing.
func testID(moduleName, coarseName, fineName, caseName string) *pb.TestIdentifier {
	result := &pb.TestIdentifier{
		ModuleName:   moduleName,
		ModuleScheme: "junit",
		ModuleVariant: &pb.Variant{
			Def: map[string]string{"key": "value"},
		},
		CoarseName: coarseName,
		FineName:   fineName,
		CaseName:   caseName,
	}
	pbutil.PopulateStructuredTestIdentifierHashes(result)
	return result
}

// longFailureReason creates a long failure reason, to allow testing truncation for users
// with limited access.
func longFailureReason() *pb.FailureReason {
	return &pb.FailureReason{
		Kind:                pb.FailureReason_CRASH,
		PrimaryErrorMessage: strings.Repeat("a", 150),
		Errors: []*pb.FailureReason_Error{
			{
				Message: strings.Repeat("a", 150),
				Trace:   "some trace",
			},
			{
				Message: strings.Repeat("b", 150),
				Trace:   "some trace2",
			},
		},
	}
}

// longSkippedReason creates a long skipped reason, to allow testing truncation for users
// with limited access.
func longSkippedReason() *pb.SkippedReason {
	return &pb.SkippedReason{
		Kind:          pb.SkippedReason_DISABLED_AT_DECLARATION,
		ReasonMessage: strings.Repeat("c", 150),
	}
}

// ExpectedVerdicts returns the expected test verdicts corresponding
// to the test data created by CreateTestData().
func ExpectedVerdicts(rootInvID rootinvocations.ID, view pb.TestVerdictView) []*pb.TestVerdict {
	makeVerdict := func(testID *pb.TestIdentifier, status pb.TestVerdict_Status, results []*pb.TestResult, exonerations []*pb.TestExoneration) *pb.TestVerdict {
		tv := &pb.TestVerdict{
			TestId:           pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier((testID))),
			TestIdStructured: testID,
			Status:           status,
			Results:          results,
			Exonerations:     exonerations,
			TestMetadata: &pb.TestMetadata{
				Name:     "tmd",
				Location: &pb.TestLocation{Repo: "https://repo", FileName: "file"},
			},
		}

		if len(exonerations) > 0 {
			tv.StatusOverride = pb.TestVerdict_EXONERATED
		} else {
			tv.StatusOverride = pb.TestVerdict_NOT_OVERRIDDEN
		}

		if view != pb.TestVerdictView_TEST_VERDICT_VIEW_FULL {
			tv.Results = nil
			tv.Exonerations = nil
			tv.TestMetadata = nil
		}

		return tv
	}

	startTime := timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	// Makes common test result parts.
	// All results have common properties.
	makeResult := func(testID *pb.TestIdentifier, resultID string, status pb.TestResult_Status) *pb.TestResult {
		r := &pb.TestResult{
			Name:                pbutil.TestResultName(string(rootInvID), "work-unit-id", pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(testID)), resultID),
			ResultId:            resultID,
			StatusV2:            status,
			StartTime:           startTime,
			Duration:            &durationpb.Duration{Nanos: 1000}, // 1000 ns = 1 us
			Tags:                []*pb.StringPair{{Key: "mytag", Value: "myvalue"}},
			SkipReason:          pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS, // Default from NewBuilder
			SkippedReason:       &pb.SkippedReason{Kind: pb.SkippedReason_DISABLED_AT_DECLARATION, ReasonMessage: "reason-message", Trace: "trace"},
			FailureReason:       &pb.FailureReason{Kind: pb.FailureReason_CRASH, PrimaryErrorMessage: "error-message", Errors: []*pb.FailureReason_Error{{Message: "error-message", Trace: "error-trace"}}},
			Properties:          &structpb.Struct{Fields: map[string]*structpb.Value{"key": structpb.NewStringValue("value")}},
			FrameworkExtensions: &pb.FrameworkExtensions{WebTest: &pb.WebTest{IsExpected: true, Status: pb.WebTest_FAIL}},
		}

		// Fix up fields based on status (like testdata.go Build() does)
		if status != pb.TestResult_FAILED {
			r.FailureReason = nil
		}
		if status != pb.TestResult_SKIPPED {
			r.SkippedReason = nil
		}

		// Status V1 mapping
		r.Status, r.Expected = pbutil.TestStatusV1FromV2(status, r.FailureReason.GetKind(), r.FrameworkExtensions.GetWebTest())

		// SummaryHTML
		r.SummaryHtml = "summary"

		return r
	}

	// Passed.
	testID1 := testID("m1", "c1", "f1", "t1")
	r1 := makeResult(testID1, "r1", pb.TestResult_PASSED)
	v1 := makeVerdict(testID1, pb.TestVerdict_PASSED, []*pb.TestResult{r1}, nil)

	// Failed.
	testID2 := testID("m1", "c1", "f1", "t2")
	r2 := makeResult(testID2, "r1", pb.TestResult_FAILED)
	r2.FailureReason = longFailureReason()
	v2 := makeVerdict(testID2, pb.TestVerdict_FAILED, []*pb.TestResult{r2}, nil)

	// Flaky.
	testID3 := testID("m1", "c1", "f1", "t3")
	r3a := makeResult(testID3, "r1", pb.TestResult_FAILED)
	r3b := makeResult(testID3, "r2", pb.TestResult_PASSED)
	v3 := makeVerdict(testID3, pb.TestVerdict_FLAKY, []*pb.TestResult{r3a, r3b}, nil)

	// Skipped.
	testID4 := testID("m1", "c1", "f1", "t4")
	r4 := makeResult(testID4, "r1", pb.TestResult_SKIPPED)
	r4.SkippedReason = longSkippedReason()
	v4 := makeVerdict(testID4, pb.TestVerdict_SKIPPED, []*pb.TestResult{r4}, nil)

	// Exonerated (Failed + Exoneration).
	testID5 := testID("m1", "c1", "f2", "t5")
	r5 := makeResult(testID5, "r1", pb.TestResult_FAILED)
	e1 := &pb.TestExoneration{
		Name:            pbutil.TestExonerationName(string(rootInvID), "testworkunit-id", pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(testID5)), "e1"),
		ExonerationId:   "e1",
		ExplanationHtml: "<b>explanation</b>",
		Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
	}
	e2 := &pb.TestExoneration{
		Name:            pbutil.TestExonerationName(string(rootInvID), "testworkunit-id", pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(testID5)), "e2"),
		ExonerationId:   "e2",
		ExplanationHtml: "<b>explanation</b>",
		Reason:          pb.ExonerationReason_NOT_CRITICAL,
	}
	v5 := makeVerdict(testID5, pb.TestVerdict_FAILED, []*pb.TestResult{r5}, []*pb.TestExoneration{e1, e2})

	// Precluded.
	testID6 := testID("m1", "c2", "f1", "t6")
	r6 := makeResult(testID6, "r1", pb.TestResult_PRECLUDED)
	v6 := makeVerdict(testID6, pb.TestVerdict_PRECLUDED, []*pb.TestResult{r6}, nil)

	// Execution Errored.
	testID7 := testID("m2", "c1", "f1", "t7")
	r7 := makeResult(testID7, "r1", pb.TestResult_EXECUTION_ERRORED)
	v7 := makeVerdict(testID7, pb.TestVerdict_EXECUTION_ERRORED, []*pb.TestResult{r7}, nil)

	return []*pb.TestVerdict{v1, v2, v3, v4, v5, v6, v7}
}

// ExpectedVerdictsMasked returns the expected test verdicts corresponding
// to the test data created by CreateTestData(), with the user having full
// access only to the given realms.
func ExpectedVerdictsMasked(rootInvID rootinvocations.ID, view pb.TestVerdictView, realms []string) []*pb.TestVerdict {
	result := ExpectedVerdicts(rootInvID, pb.TestVerdictView_TEST_VERDICT_VIEW_FULL)
	for _, tv := range result {
		hasFullAccessToAny := false
		for _, r := range tv.Results {
			resultRealm := fmt.Sprintf("testproject:%s-%s", tv.TestIdStructured.CaseName, r.ResultId)
			if slices.Contains(realms, resultRealm) {
				hasFullAccessToAny = true
				continue
			}

			r.Tags = nil
			r.Properties = nil
			r.SummaryHtml = ""

			// Truncate FailureReason.
			if r.FailureReason != nil {
				r.FailureReason.PrimaryErrorMessage = pbutil.TruncateString(r.FailureReason.PrimaryErrorMessage, 140)
				for _, e := range r.FailureReason.Errors {
					e.Message = pbutil.TruncateString(e.Message, 140)
					e.Trace = ""
				}
			}
			// Truncate SkippedReason.
			if r.SkippedReason != nil {
				r.SkippedReason.ReasonMessage = pbutil.TruncateString(r.SkippedReason.ReasonMessage, 140)
				r.SkippedReason.Trace = ""
			}
			r.IsMasked = true
		}
		if !hasFullAccessToAny {
			tv.TestIdStructured.ModuleVariant = nil
			tv.TestMetadata = nil
			tv.IsMasked = true
		}
		if view != pb.TestVerdictView_TEST_VERDICT_VIEW_FULL {
			tv.Results = nil
			tv.Exonerations = nil
			tv.TestMetadata = nil
		}
	}
	return result
}
