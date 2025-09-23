// Copyright 2022 The LUCI Authors.
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

package stability

import (
	"context"
	"time"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var referenceTime = time.Date(2022, time.June, 17, 8, 0, 0, 0, time.UTC)

// CreateQueryStabilityTestData creates test data in Spanner for testing
// QueryStability.
func CreateQueryStabilityTestData(ctx context.Context) error {
	var1 := pbutil.Variant("key1", "val1", "key2", "val1")
	var2 := pbutil.Variant("key1", "val2", "key2", "val1")
	var3 := pbutil.Variant("key1", "val2", "key2", "val2")

	onBranch := pbutil.SourceRefHash(&pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{
				Host:    "mysources.googlesource.com",
				Project: "myproject/src",
				Ref:     "refs/heads/mybranch",
			},
		},
	})
	otherBranch := pbutil.SourceRefHash(&pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{
				Host:    "mysources.googlesource.com",
				Project: "myproject/src",
				Ref:     "refs/heads/otherbranch",
			},
		},
	})

	type verdict struct {
		partitionTime time.Time
		variant       *pb.Variant
		invocationID  string
		runStatuses   []lowlatency.RunStatus
		sources       testresults.Sources
	}

	changelists := func(clOwnerKind pb.ChangelistOwnerKind, numbers ...int64) []testresults.Changelist {
		var changelists []testresults.Changelist
		for _, clNum := range numbers {
			changelists = append(changelists, testresults.Changelist{
				Host:      "mygerrit-review.googlesource.com",
				Change:    clNum,
				Patchset:  5,
				OwnerKind: clOwnerKind,
			})
		}
		return changelists
	}

	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// pass, fail is shorthand here for expected and unexpected run,
		// where for the purposes of this RPC, a flaky run counts as
		// "expected" (as it has at least one expected result).
		failPass := []lowlatency.RunStatus{lowlatency.Unexpected, lowlatency.Flaky}
		failPassPass := []lowlatency.RunStatus{lowlatency.Unexpected, lowlatency.Flaky, lowlatency.Flaky}
		pass := []lowlatency.RunStatus{lowlatency.Flaky}
		fail := []lowlatency.RunStatus{lowlatency.Unexpected}
		failFail := []lowlatency.RunStatus{lowlatency.Unexpected, lowlatency.Unexpected}

		day := 24 * time.Hour

		automationOwner := pb.ChangelistOwnerKind_AUTOMATION
		humanOwner := pb.ChangelistOwnerKind_HUMAN
		unspecifiedOwner := pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED

		verdicts := []verdict{
			{
				partitionTime: referenceTime.Add(-14*day - time.Microsecond),
				variant:       var1,
				// Partition time too early, so should be ignored.
				invocationID: "sourceverdict0-ignore-partitiontime",
				runStatuses:  failPass,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 1,
				},
			},
			{
				partitionTime: referenceTime.Add(-13*day - 1*time.Hour),
				variant:       var1,
				invocationID:  "sourceverdict1",
				runStatuses:   failPass,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 90,
				},
			},
			{
				partitionTime: referenceTime.Add(-12 * day),
				variant:       var1,
				invocationID:  "sourceverdict2",
				runStatuses:   failPass,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 100,
				},
			},
			{
				partitionTime: referenceTime.Add(-10 * day),
				variant:       var1,
				invocationID:  "sourceverdict3-part1",
				runStatuses:   fail,
				sources: testresults.Sources{
					RefHash:     onBranch,
					Position:    100,
					Changelists: changelists(humanOwner, 1),
				},
			},
			{
				partitionTime: referenceTime.Add(-9 * day),
				variant:       var1,
				// Should combine with sourceverdict3-part1 to produce a run-flaky verdict, as it
				// has the same sources under test.
				invocationID: "sourceverdict3-part2",
				runStatuses:  pass,
				sources: testresults.Sources{
					RefHash:     onBranch,
					Position:    100,
					Changelists: changelists(humanOwner, 1),
				},
			},
			{
				partitionTime: referenceTime.Add(-8*day - 3*time.Hour),
				variant:       var1,
				// source verdict 3 should be used preferentially as it is flaky
				// and tests the same CL.
				invocationID: "sourceverdict4-ignore",
				runStatuses:  pass,
				sources: testresults.Sources{
					RefHash:     onBranch,
					Position:    120,
					Changelists: changelists(humanOwner, 1),
				},
			},
			{
				partitionTime: referenceTime.Add(-8*day - 2*time.Hour),
				variant:       var1,
				invocationID:  "sourceverdict5",
				runStatuses:   pass,
				sources: testresults.Sources{
					RefHash:     onBranch,
					Position:    120,
					Changelists: changelists(unspecifiedOwner, 2),
				},
			},
			{
				partitionTime: referenceTime.Add(-8*day - time.Hour),
				variant:       var1,
				invocationID:  "sourceverdict6",
				runStatuses:   failPass,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 120,
					IsDirty:  true,
				},
			},
			{
				partitionTime: referenceTime.Add(-8 * day),
				variant:       var1,
				// Should be distinct from source verdict 5 because both have
				// IsDirty set, so we cannot confirm the soruces are identical.
				invocationID: "sourceverdict7",
				runStatuses:  failPassPass,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 120,
					IsDirty:  true,
				},
			},
			{
				partitionTime: referenceTime.Add(-8 * day),
				variant:       var1,
				// Automation-authored, so should be ignored.
				invocationID: "sourceverdict8-ignore",
				runStatuses:  failPass,
				sources: testresults.Sources{
					RefHash:     onBranch,
					Position:    120,
					Changelists: changelists(automationOwner, 4),
				},
			},
			{
				partitionTime: referenceTime.Add(-7*day - 1*time.Hour),
				variant:       var1,
				invocationID:  "sourceverdict9-part1",
				runStatuses:   failFail,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 128,
				},
			},
			{
				partitionTime: referenceTime.Add(-7*day - 1*time.Hour),
				variant:       var1,
				// Should merge with sourceverdict8-part1 due to sharing
				// same sources.
				invocationID: "sourceverdict9-part2",
				runStatuses:  pass,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 128,
				},
			},
			{
				partitionTime: referenceTime.Add(-7 * day),
				variant:       var2,
				// Different variant, so should be ignored.
				invocationID: "sourceverdict10-ignore-var2",
				runStatuses:  failPass,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 130,
				},
			},
			{
				partitionTime: referenceTime.Add(-7 * day),
				variant:       var3,
				// For variant 3.
				invocationID: "sourceverdict11",
				runStatuses:  failPass,
				sources: testresults.Sources{
					RefHash:  otherBranch,
					Position: 130,
				},
			},
			{
				partitionTime: referenceTime.Add(-7 * day),
				variant:       var1,
				// Other branch, so should be ignored.
				invocationID: "sourceverdict12-ignore-offbranch",
				runStatuses:  failPass,
				sources: testresults.Sources{
					RefHash:  otherBranch,
					Position: 130,
				},
			},
			{
				partitionTime: referenceTime.Add(-7 * day),
				variant:       var1,
				// A result for the same CL as being queried, so should be ignored
				// to avoid CLs contributing to their own exoneration.
				invocationID: "sourceverdict14-ignore-same-cl-as-query",
				runStatuses:  failPass,
				sources: testresults.Sources{
					RefHash:     otherBranch,
					Position:    130,
					Changelists: changelists(pb.ChangelistOwnerKind_HUMAN, 888888),
				},
			},
			{
				partitionTime: referenceTime.Add(-6 * day),
				variant:       var1,
				invocationID:  "sourceverdict15",
				runStatuses:   pass,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 129,
				},
			},
			// Begin consecutive failure streak.
			{
				partitionTime: referenceTime.Add(-6 * day),
				variant:       var1,
				invocationID:  "sourceverdict16-part1",
				runStatuses:   failFail,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 130,
				},
			},
			{
				partitionTime: referenceTime.Add(-6 * day),
				variant:       var1,
				// Should merge with sourceverdict13-part1 due to sharing
				// same sources.
				invocationID: "sourceverdict16-part2",
				runStatuses:  failFail,
				sources: testresults.Sources{
					RefHash:  onBranch,
					Position: 130,
				},
			},
			{
				partitionTime: referenceTime.Add(-1 * day),
				variant:       var1,
				invocationID:  "sourceverdict17",
				// Only one run should be used as the verdict relates to presubmit testing.
				runStatuses: failPass,
				sources: testresults.Sources{
					RefHash:     onBranch,
					Changelists: changelists(pb.ChangelistOwnerKind_HUMAN, 888777),
					Position:    140,
				},
			},
		}

		for _, v := range verdicts {
			baseTestResult := lowlatency.NewTestResult().
				WithProject("project").
				WithTestID("test_id").
				WithVariantHash(pbutil.VariantHash(v.variant)).
				WithPartitionTime(v.partitionTime).
				WithRootInvocationID(v.invocationID).
				WithSubRealm("realm").
				WithStatus(pb.TestResultStatus_PASS).
				WithSources(v.sources)

			trs := lowlatency.NewTestVerdict().
				WithBaseTestResult(baseTestResult.Build()).
				WithRunStatus(v.runStatuses...).
				Build()
			for _, tr := range trs {
				span.BufferWrite(ctx, tr.SaveUnverified())
			}
		}

		return nil
	})
	return err
}

func QueryStabilitySampleRequest() QueryStabilityOptions {
	var1 := pbutil.Variant("key1", "val1", "key2", "val1")
	var3 := pbutil.Variant("key1", "val2", "key2", "val2")
	testVariants := []*pb.QueryTestVariantStabilityRequest_TestVariantPosition{
		{
			TestId:  "test_id",
			Variant: var1,
			Sources: &pb.Sources{
				BaseSources: &pb.Sources_GitilesCommit{
					GitilesCommit: &pb.GitilesCommit{
						Host:       "mysources.googlesource.com",
						Project:    "myproject/src",
						Ref:        "refs/heads/mybranch",
						CommitHash: "aabbccddeeff00112233aabbccddeeff00112233",
						Position:   130,
					},
				},
				Changelists: []*pb.GerritChange{
					{
						Host:     "mygerrit-review.googlesource.com",
						Project:  "mygerrit/src",
						Change:   888888,
						Patchset: 1,
					},
				},
			},
		},
		{
			TestId:  "test_id",
			Variant: var3,
			Sources: &pb.Sources{
				BaseSources: &pb.Sources_GitilesCommit{
					GitilesCommit: &pb.GitilesCommit{
						Host:       "mysources.googlesource.com",
						Project:    "myproject/src",
						Ref:        "refs/heads/otherbranch",
						CommitHash: "00bbccddeeff00112233aabbccddeeff00112233",
						Position:   120,
					},
				},
			},
		},
	}
	return QueryStabilityOptions{
		Project:              "project",
		SubRealms:            []string{"realm"},
		TestVariantPositions: testVariants,
		Criteria:             CreateSampleStabilityCriteria(),
		AsAtTime:             time.Date(2022, time.June, 17, 8, 0, 0, 0, time.UTC),
	}
}

func CreateSampleStabilityCriteria() *pb.TestStabilityCriteria {
	return &pb.TestStabilityCriteria{
		FailureRate: &pb.TestStabilityCriteria_FailureRateCriteria{
			FailureThreshold:            7,
			ConsecutiveFailureThreshold: 3,
		},
		FlakeRate: &pb.TestStabilityCriteria_FlakeRateCriteria{
			// Use 5 instead of a more typical value like 100 because of the
			// limited sample data.
			MinWindow:          5,
			FlakeThreshold:     2,
			FlakeRateThreshold: 0.01,
			FlakeThreshold_1Wd: 1,
		},
	}
}

// QueryStabilitySampleResponse returns expected response data from QueryFailureRate
// after being invoked with QueryFailureRateSampleRequest.
// It is assumed test data was setup with CreateQueryFailureRateTestData.
func QueryStabilitySampleResponse() []*pb.TestVariantStabilityAnalysis {
	var1 := pbutil.Variant("key1", "val1", "key2", "val1")
	var3 := pbutil.Variant("key1", "val2", "key2", "val2")

	analysis := []*pb.TestVariantStabilityAnalysis{
		{
			TestId:  "test_id",
			Variant: var1,
			FailureRate: &pb.TestVariantStabilityAnalysis_FailureRate{
				IsMet:                         true,
				UnexpectedTestRuns:            8,
				ConsecutiveUnexpectedTestRuns: 5,
				RecentVerdicts: []*pb.TestVariantStabilityAnalysis_FailureRate_RecentVerdict{
					{
						Position: 140,
						Changelists: []*pb.Changelist{
							{
								Host:      "mygerrit-review.googlesource.com",
								Change:    888777,
								Patchset:  5,
								OwnerKind: pb.ChangelistOwnerKind_HUMAN,
							},
						},
						Invocations: []string{"sourceverdict17"},
						// Unexpected + expected run flattened down to only an unexpected run due to
						// presubmit results only being allowed to contribute 1 test run.
						UnexpectedRuns: 1,
						TotalRuns:      1,
					},
					{
						Position:       130, // Query position.
						Invocations:    []string{"sourceverdict16-part1", "sourceverdict16-part2"},
						UnexpectedRuns: 4,
						TotalRuns:      4,
					},
					{
						Position:    129,
						Invocations: []string{"sourceverdict15"},
						TotalRuns:   1,
					},
					{
						Position:       128,
						Invocations:    []string{"sourceverdict9-part1", "sourceverdict9-part2"},
						UnexpectedRuns: 2,
						TotalRuns:      3,
					},
					{
						Position:       120,
						Invocations:    []string{"sourceverdict7"},
						UnexpectedRuns: 1,
						TotalRuns:      2, // Verdict truncated to 2 runs to keep total runs on or before query position <= 10.
					},
				},
			},
			FlakeRate: &pb.TestVariantStabilityAnalysis_FlakeRate{
				IsMet:            true,
				TotalVerdicts:    9,
				RunFlakyVerdicts: 6,
				FlakeExamples: []*pb.TestVariantStabilityAnalysis_FlakeRate_VerdictExample{
					{
						Position: 140,
						Changelists: []*pb.Changelist{
							{
								Host:      "mygerrit-review.googlesource.com",
								Change:    888777,
								Patchset:  5,
								OwnerKind: pb.ChangelistOwnerKind_HUMAN,
							},
						},
						Invocations: []string{"sourceverdict17"},
					},
					{
						Position:    128,
						Invocations: []string{"sourceverdict9-part1", "sourceverdict9-part2"},
					},
					{
						Position:    120,
						Invocations: []string{"sourceverdict7"},
					},
					{
						Position:    120,
						Invocations: []string{"sourceverdict6"},
					},
					{
						Position:    100,
						Invocations: []string{"sourceverdict3-part1", "sourceverdict3-part2"},
						Changelists: []*pb.Changelist{
							{
								Host:      "mygerrit-review.googlesource.com",
								Change:    1,
								Patchset:  5,
								OwnerKind: pb.ChangelistOwnerKind_HUMAN,
							},
						},
					},
					{
						Position:    100,
						Invocations: []string{"sourceverdict2"},
					},
				},
				StartPosition: 100,
				EndPosition:   140,

				RunFlakyVerdicts_1Wd: 1,
				RunFlakyVerdicts_12H: 0,
				StartPosition_1Wd:    128,
				EndPosition_1Wd:      130,
			},
		},
		{
			TestId:  "test_id",
			Variant: var3,
			FailureRate: &pb.TestVariantStabilityAnalysis_FailureRate{
				IsMet:                         false,
				UnexpectedTestRuns:            1,
				ConsecutiveUnexpectedTestRuns: 1,
				RecentVerdicts: []*pb.TestVariantStabilityAnalysis_FailureRate_RecentVerdict{
					{
						Position:       130,
						Invocations:    []string{"sourceverdict11"},
						UnexpectedRuns: 1,
						TotalRuns:      2,
					},
				},
			},
			FlakeRate: &pb.TestVariantStabilityAnalysis_FlakeRate{
				IsMet:            false,
				RunFlakyVerdicts: 1,
				TotalVerdicts:    1,
				FlakeExamples: []*pb.TestVariantStabilityAnalysis_FlakeRate_VerdictExample{
					{
						Position:    130,
						Invocations: []string{"sourceverdict11"},
					},
				},
				StartPosition: 130,
				EndPosition:   130,

				RunFlakyVerdicts_1Wd: 1,
				RunFlakyVerdicts_12H: 1,
				StartPosition_1Wd:    130,
				EndPosition_1Wd:      130,
			},
		},
	}
	return analysis
}

// QueryStabilitySampleResponseLargeWindow is the expected response from
// QueryStability for FlakeRate.MinWindow = 100.
func QueryStabilitySampleResponseLargeWindow() []*pb.TestVariantStabilityAnalysis {
	rsp := QueryStabilitySampleResponse()
	fr := rsp[0].FlakeRate
	fr.TotalVerdicts++
	fr.RunFlakyVerdicts++

	// Include verdicts outside the normal window, but within the last 14 days.
	fr.FlakeExamples = append(fr.FlakeExamples, &pb.TestVariantStabilityAnalysis_FlakeRate_VerdictExample{
		Position:    90,
		Invocations: []string{"sourceverdict1"},
	})
	fr.StartPosition = 90

	return rsp
}
