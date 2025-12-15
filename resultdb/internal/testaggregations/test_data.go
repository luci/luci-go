// Copyright 2025 The LUCI Authors.
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

package testaggregations

import (
	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// CreateTestData creates test data used for testing test aggregations.
func CreateTestData(rootInvID rootinvocations.ID) []*spanner.Mutation {
	moduleID := func(id, scheme string) *pb.ModuleIdentifier {
		return &pb.ModuleIdentifier{
			ModuleName:    id,
			ModuleVariant: pbutil.Variant("key", "value"),
			ModuleScheme:  scheme,
		}
	}

	// Prepare work units.
	workUnits := []*workunits.WorkUnitRow{
		// M1: Should be marked succeeded since at least one attempt in the only shard succeeded.
		// This is despite an earlier failure, skip, cancellation, and a pending and running attempt.
		workunits.NewBuilder(rootInvID, "wu-m1-a1").WithModuleID(moduleID("m1", "junit")).WithState(pb.WorkUnit_FAILED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m1-a2").WithModuleID(moduleID("m1", "junit")).WithState(pb.WorkUnit_SUCCEEDED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m1-a3").WithModuleID(moduleID("m1", "junit")).WithState(pb.WorkUnit_SKIPPED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m1-a4").WithModuleID(moduleID("m1", "junit")).WithState(pb.WorkUnit_CANCELLED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m1-a5").WithModuleID(moduleID("m1", "junit")).WithState(pb.WorkUnit_PENDING).Build(),
		workunits.NewBuilder(rootInvID, "wu-m1-a6").WithModuleID(moduleID("m1", "junit")).WithState(pb.WorkUnit_RUNNING).Build(),
		// M2: Should be marked running since a retry is in progress.
		workunits.NewBuilder(rootInvID, "wu-m2-a1").WithModuleID(moduleID("m2", "noconfig")).WithState(pb.WorkUnit_FAILED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m2-a2").WithModuleID(moduleID("m2", "noconfig")).WithState(pb.WorkUnit_RUNNING).Build(),
		// M3: Should be marked failed since one shard failed. This is despite one succeeding,
		// another still being in progress, one being cancelled, one being pending, and one
		// being skipped.
		workunits.NewBuilder(rootInvID, "wu-m3-s1").WithModuleID(moduleID("m3", "flat")).WithModuleShardKey("s1").WithState(pb.WorkUnit_FAILED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m3-s2").WithModuleID(moduleID("m3", "flat")).WithModuleShardKey("s2").WithState(pb.WorkUnit_RUNNING).Build(),
		workunits.NewBuilder(rootInvID, "wu-m3-s3").WithModuleID(moduleID("m3", "flat")).WithModuleShardKey("s3").WithState(pb.WorkUnit_SKIPPED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m3-s4").WithModuleID(moduleID("m3", "flat")).WithModuleShardKey("s4").WithState(pb.WorkUnit_SUCCEEDED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m3-s5").WithModuleID(moduleID("m3", "flat")).WithModuleShardKey("s5").WithState(pb.WorkUnit_CANCELLED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m3-s6").WithModuleID(moduleID("m3", "flat")).WithModuleShardKey("s6").WithState(pb.WorkUnit_PENDING).Build(),
		// M4: Should be marked pending, despite an earlier failure.
		workunits.NewBuilder(rootInvID, "wu-m4-a1").WithModuleID(moduleID("m4", "junit")).WithState(pb.WorkUnit_PENDING).Build(),
		workunits.NewBuilder(rootInvID, "wu-m4-a2").WithModuleID(moduleID("m4", "junit")).WithState(pb.WorkUnit_FAILED).Build(),
		// M5: Should be marked skipped.
		workunits.NewBuilder(rootInvID, "wu-m5").WithModuleID(moduleID("m5", "junit")).WithState(pb.WorkUnit_SKIPPED).Build(),
		// M6: Should be marked cancelled, despite another shard succeeding.
		workunits.NewBuilder(rootInvID, "wu-m6-s1").WithModuleID(moduleID("m6", "junit")).WithModuleShardKey("s1").WithState(pb.WorkUnit_CANCELLED).Build(),
		workunits.NewBuilder(rootInvID, "wu-m6-s2").WithModuleID(moduleID("m6", "junit")).WithModuleShardKey("s2").WithState(pb.WorkUnit_SUCCEEDED).Build(),
	}

	// Prepare test results.

	// Pick a few shards. In reality each test may be in a different shard.
	shard := rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 5}
	shard2 := rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 11}
	baseBuilder := func() *testresultsv2.Builder {
		return testresultsv2.NewBuilder().WithRootInvocationShardID(shard).
			WithModuleName("m1").WithModuleScheme("junit").WithModuleVariant(pbutil.Variant("key", "value")).WithCoarseName("c1").WithFineName("f1").WithCaseName("t1")
	}
	baseExonerationBuilder := func() *testexonerationsv2.Builder {
		return testexonerationsv2.NewBuilder().WithRootInvocationShardID(shard).
			WithModuleName("m1").WithModuleScheme("junit").WithModuleVariant(pbutil.Variant("key", "value")).WithCoarseName("c1").WithFineName("f1").WithCaseName("t1")
	}

	// Scenario 1: Passed, Failed, Exonerated, Flaky, Skipped, Error, Precluded
	results := []*testresultsv2.TestResultRow{
		// Passed
		baseBuilder().WithCaseName("passed_test").WithStatusV2(pb.TestResult_PASSED).Build(),
		// Failed
		baseBuilder().WithCaseName("failed_test").WithStatusV2(pb.TestResult_FAILED).Build(),
		// Exonerated
		baseBuilder().WithFineName("f2").WithCaseName("exonerated_test").WithStatusV2(pb.TestResult_FAILED).Build(),
		// Flaky (in another shard)
		baseBuilder().WithRootInvocationShardID(shard2).WithFineName("f3").WithCaseName("flaky_test").WithResultID("fr1").WithStatusV2(pb.TestResult_PASSED).Build(),
		baseBuilder().WithRootInvocationShardID(shard2).WithFineName("f3").WithCaseName("flaky_test").WithResultID("fr2").WithStatusV2(pb.TestResult_FAILED).Build(),
		// Skipped
		baseBuilder().WithCoarseName("c2").WithCaseName("skipped_test").WithStatusV2(pb.TestResult_SKIPPED).Build(),
		// Execution Errored
		baseBuilder().WithModuleName("m2").WithModuleScheme("noconfig").WithCaseName("execution_errored_test").WithStatusV2(pb.TestResult_EXECUTION_ERRORED).Build(),
		// Precluded
		baseBuilder().WithModuleName("m3").WithModuleScheme("flat").WithCoarseName("").WithFineName("").WithCaseName("precluded_test").WithStatusV2(pb.TestResult_PRECLUDED).Build(),
	}
	exonerations := []*testexonerationsv2.TestExonerationRow{
		// Exonerate one of the failed tests
		baseExonerationBuilder().WithFineName("f2").WithCaseName("exonerated_test").Build(),
	}

	// Prepare mutations.
	var ms []*spanner.Mutation
	for _, r := range workUnits {
		ms = append(ms, workunits.InsertForTesting(r)...)
	}
	for _, r := range results {
		ms = append(ms, testresultsv2.InsertForTesting(r))
	}
	for _, e := range exonerations {
		ms = append(ms, testexonerationsv2.InsertForTesting(e))
	}
	return ms
}

func ExpectedRootInvocationAggregation() *pb.TestAggregation {
	return &pb.TestAggregation{
		Id: &pb.TestIdentifierPrefix{
			Level: pb.AggregationLevel_INVOCATION,
			Id:    &pb.TestIdentifier{},
		},
		NextFinerLevel: pb.AggregationLevel_MODULE,
		VerdictCounts: &pb.TestAggregation_VerdictCounts{
			Failed:               1,
			Flaky:                1,
			Passed:               1,
			Skipped:              1,
			ExecutionErrored:     1,
			Precluded:            1,
			Exonerated:           1,
			FailedBase:           2,
			FlakyBase:            1,
			PassedBase:           1,
			SkippedBase:          1,
			ExecutionErroredBase: 1,
			PrecludedBase:        1,
		},
		ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{
			Failed:    1,
			Running:   1,
			Pending:   1,
			Cancelled: 1,
			Succeeded: 1,
			Skipped:   1,
		},
	}
}

func ExpectedModuleAggregationsIDOrder() []*pb.TestAggregation {
	return []*pb.TestAggregation{
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_MODULE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m1",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
				},
			},
			NextFinerLevel: pb.AggregationLevel_COARSE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Failed:      1,
				Flaky:       1,
				Passed:      1,
				Skipped:     1,
				Exonerated:  1,
				FailedBase:  2,
				FlakyBase:   1,
				PassedBase:  1,
				SkippedBase: 1,
			},
			ModuleStatus: pb.TestAggregation_SUCCEEDED,
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_MODULE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m2",
					ModuleScheme:      "noconfig",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
				},
			},
			NextFinerLevel: pb.AggregationLevel_COARSE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				ExecutionErrored:     1,
				ExecutionErroredBase: 1,
			},
			ModuleStatus: pb.TestAggregation_RUNNING,
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_MODULE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m3",
					ModuleScheme:      "flat",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
				},
			},
			NextFinerLevel: pb.AggregationLevel_CASE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Precluded:     1,
				PrecludedBase: 1,
			},
			ModuleStatus: pb.TestAggregation_ERRORED,
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_MODULE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m4",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
				},
			},
			NextFinerLevel: pb.AggregationLevel_COARSE,
			VerdictCounts:  &pb.TestAggregation_VerdictCounts{},
			ModuleStatus:   pb.TestAggregation_PENDING,
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_MODULE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m5",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
				},
			},
			NextFinerLevel: pb.AggregationLevel_COARSE,
			VerdictCounts:  &pb.TestAggregation_VerdictCounts{},
			ModuleStatus:   pb.TestAggregation_SKIPPED,
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_MODULE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m6",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
				},
			},
			NextFinerLevel: pb.AggregationLevel_COARSE,
			VerdictCounts:  &pb.TestAggregation_VerdictCounts{},
			ModuleStatus:   pb.TestAggregation_CANCELLED,
		},
	}
}

func ExpectedModuleAggregationsUIOrder() []*pb.TestAggregation {
	result := ExpectedModuleAggregationsIDOrder()
	// Cancelled sorts before pending and skipped.
	result[3], result[4], result[5] = result[5], result[3], result[4]
	return result
}

func ExpectedCoarseAggregationsIDOrder() []*pb.TestAggregation {
	return []*pb.TestAggregation{
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_COARSE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m1",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "c1",
				},
			},
			NextFinerLevel: pb.AggregationLevel_FINE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Failed:     1,
				Flaky:      1,
				Passed:     1,
				Exonerated: 1,
				FailedBase: 2,
				FlakyBase:  1,
				PassedBase: 1,
			},
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_COARSE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m1",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "c2",
				},
			},
			NextFinerLevel: pb.AggregationLevel_FINE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Skipped:     1,
				SkippedBase: 1,
			},
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_COARSE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m2",
					ModuleScheme:      "noconfig",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "c1",
				},
			},
			NextFinerLevel: pb.AggregationLevel_FINE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				ExecutionErrored:     1,
				ExecutionErroredBase: 1,
			},
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_COARSE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m3",
					ModuleScheme:      "flat",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "",
				},
			},
			NextFinerLevel: pb.AggregationLevel_CASE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Precluded:     1,
				PrecludedBase: 1,
			},
		},
	}
}

func ExpectedCoarseAggregationsUIOrder() []*pb.TestAggregation {
	// Modules with failures sort first, then execution errored and precluded.
	result := ExpectedCoarseAggregationsIDOrder()
	result[1], result[2], result[3] = result[2], result[3], result[1]
	return result
}

func ExpectedFineAggregationsIDOrder() []*pb.TestAggregation {
	return []*pb.TestAggregation{
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_FINE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m1",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "c1",
					FineName:          "f1",
				},
			},
			NextFinerLevel: pb.AggregationLevel_CASE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Failed:     1,
				Passed:     1,
				FailedBase: 1,
				PassedBase: 1,
			},
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_FINE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m1",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "c1",
					FineName:          "f2",
				},
			},
			NextFinerLevel: pb.AggregationLevel_CASE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Exonerated: 1,
				FailedBase: 1,
			},
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_FINE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m1",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "c1",
					FineName:          "f3",
				},
			},
			NextFinerLevel: pb.AggregationLevel_CASE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Flaky:     1,
				FlakyBase: 1,
			},
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_FINE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m1",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "c2",
					FineName:          "f1",
				},
			},
			NextFinerLevel: pb.AggregationLevel_CASE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Skipped:     1,
				SkippedBase: 1,
			},
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_FINE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m2",
					ModuleScheme:      "noconfig",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "c1",
					FineName:          "f1",
				},
			},
			NextFinerLevel: pb.AggregationLevel_CASE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				ExecutionErrored:     1,
				ExecutionErroredBase: 1,
			},
		},
		{
			Id: &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_FINE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m3",
					ModuleScheme:      "flat",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "",
					FineName:          "",
				},
			},
			NextFinerLevel: pb.AggregationLevel_CASE,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				Precluded:     1,
				PrecludedBase: 1,
			},
		},
	}
}

func ExpectedFineAggregationsUIOrder() []*pb.TestAggregation {
	result := ExpectedFineAggregationsIDOrder()
	// Modules with failures sort first, then execution errored and precluded,
	// then flaky, then exonerations.
	result[1], result[2], result[3], result[4], result[5] = result[4], result[5], result[2], result[1], result[3]
	return result
}
