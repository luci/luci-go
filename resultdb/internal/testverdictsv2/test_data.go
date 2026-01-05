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
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
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
			WithTestMetadata(&pb.TestMetadata{Name: "tmd", Location: &pb.TestLocation{Repo: "https://repo", FileName: "file"}})
	}
	baseExonerationBuilder := func() *testexonerationsv2.Builder {
		return testexonerationsv2.NewBuilder().WithRootInvocationShardID(shard).
			WithModuleName("m1").WithModuleScheme("junit").WithModuleVariant(pbutil.Variant("key", "value")).WithCoarseName("c1").WithFineName("f1").WithCaseName("t1")
	}

	results := []*testresultsv2.TestResultRow{
		// Verdict 1: Passed
		// t1
		baseBuilder().WithResultID("r1").WithCaseName("t1").WithStatusV2(pb.TestResult_PASSED).Build(),

		// Verdict 2: Failed
		// t2
		baseBuilder().WithResultID("r1").WithCaseName("t2").WithStatusV2(pb.TestResult_FAILED).Build(),

		// Verdict 3: Flaky (Pass + Fail)
		// t3
		baseBuilder().WithResultID("r1").WithCaseName("t3").WithStatusV2(pb.TestResult_FAILED).Build(),
		baseBuilder().WithResultID("r2").WithCaseName("t3").WithStatusV2(pb.TestResult_PASSED).Build(),

		// Verdict 4: Skipped
		// t4
		baseBuilder().WithResultID("r1").WithCaseName("t4").WithStatusV2(pb.TestResult_SKIPPED).Build(),

		// Verdict 5: Exonerated (Fail + Exoneration)
		// t5
		baseBuilder().WithResultID("r1").WithCaseName("t5").WithStatusV2(pb.TestResult_FAILED).Build(),

		// Verdict 6: Precluded
		// t6
		baseBuilder().WithResultID("r1").WithCaseName("t6").WithStatusV2(pb.TestResult_PRECLUDED).Build(),

		// Verdict 7: Execution Errored
		// t7
		baseBuilder().WithResultID("r1").WithCaseName("t7").WithStatusV2(pb.TestResult_EXECUTION_ERRORED).Build(),
	}

	exonerations := []*testexonerationsv2.TestExonerationRow{
		// Exonerate t5
		baseExonerationBuilder().WithCaseName("t5").WithExonerationID("e1").WithReason(pb.ExonerationReason_OCCURS_ON_OTHER_CLS).Build(),
	}

	// Prepare mutations.
	var ms []*spanner.Mutation
	for _, r := range results {
		ms = append(ms, testresultsv2.InsertForTesting(r))
	}
	for _, e := range exonerations {
		ms = append(ms, testexonerationsv2.InsertForTesting(e))
	}
	return ms
}

// flatTestID returns the test ID of the given case name. It matches
// CreateTestData logic.
func flatTestID(rootInvID rootinvocations.ID, caseName string) string {
	return pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(&pb.TestIdentifier{
		ModuleName:   "m1",
		ModuleScheme: "junit",
		ModuleVariant: &pb.Variant{
			Def: map[string]string{"key": "value"},
		},
		CoarseName: "c1",
		FineName:   "f1",
		CaseName:   caseName,
	}))
}

// ExpectedVerdicts returns the expected test verdicts corresponding
// to the test data created by CreateTestData().
func ExpectedVerdicts(rootInvID rootinvocations.ID) []*pb.TestVerdict {
	// Base values matching test_data.go defaults
	moduleName := "m1"
	moduleScheme := "junit"
	moduleVariant := pbutil.Variant("key", "value")
	moduleVariantHash := pbutil.VariantHash(moduleVariant)
	coarseName := "c1"
	fineName := "f1"

	makeVerdict := func(caseName string, status pb.TestVerdict_Status, results []*pb.TestResult, exonerations []*pb.TestExoneration) *pb.TestVerdict {
		tv := &pb.TestVerdict{
			TestId: flatTestID(rootInvID, caseName),
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:        moduleName,
				ModuleScheme:      moduleScheme,
				ModuleVariant:     moduleVariant,
				ModuleVariantHash: moduleVariantHash,
				CoarseName:        coarseName,
				FineName:          fineName,
				CaseName:          caseName,
			},
			Status:       status,
			Results:      results,
			Exonerations: exonerations,
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

		return tv
	}

	startTime := timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	// Makes common test result parts.
	// All results have common properties.
	makeResult := func(caseName, resultID string, status pb.TestResult_Status) *pb.TestResult {
		r := &pb.TestResult{
			Name:                pbutil.TestResultName(string(rootInvID), "work-unit-id", flatTestID(rootInvID, caseName), resultID),
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
	r1 := makeResult("t1", "r1", pb.TestResult_PASSED)
	v1 := makeVerdict("t1", pb.TestVerdict_PASSED, []*pb.TestResult{r1}, nil)

	// Failed.
	r2 := makeResult("t2", "r1", pb.TestResult_FAILED)
	v2 := makeVerdict("t2", pb.TestVerdict_FAILED, []*pb.TestResult{r2}, nil)

	// Flaky.
	r3a := makeResult("t3", "r1", pb.TestResult_FAILED)
	r3b := makeResult("t3", "r2", pb.TestResult_PASSED)
	v3 := makeVerdict("t3", pb.TestVerdict_FLAKY, []*pb.TestResult{r3a, r3b}, nil)

	// Skipped.
	r4 := makeResult("t4", "r1", pb.TestResult_SKIPPED)
	v4 := makeVerdict("t4", pb.TestVerdict_SKIPPED, []*pb.TestResult{r4}, nil)

	// Exonerated (Failed + Exoneration).
	r5 := makeResult("t5", "r1", pb.TestResult_FAILED)
	e5 := &pb.TestExoneration{
		Name:            pbutil.TestExonerationName(string(rootInvID), "testworkunit-id", flatTestID(rootInvID, "t5"), "e1"),
		ExonerationId:   "e1",
		ExplanationHtml: "<b>explanation</b>",
		Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
	}
	v5 := makeVerdict("t5", pb.TestVerdict_FAILED, []*pb.TestResult{r5}, []*pb.TestExoneration{e5})

	// Precluded.
	r6 := makeResult("t6", "r1", pb.TestResult_PRECLUDED)
	v6 := makeVerdict("t6", pb.TestVerdict_PRECLUDED, []*pb.TestResult{r6}, nil)

	// Execution Errored.
	r7 := makeResult("t7", "r1", pb.TestResult_EXECUTION_ERRORED)
	v7 := makeVerdict("t7", pb.TestVerdict_EXECUTION_ERRORED, []*pb.TestResult{r7}, nil)

	return []*pb.TestVerdict{v1, v2, v3, v4, v5, v6, v7}
}
