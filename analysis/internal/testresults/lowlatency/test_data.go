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

package lowlatency

import (
	"strings"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var TestRef = &pb.SourceRef{
	System: &pb.SourceRef_Gitiles{
		Gitiles: &pb.GitilesRef{
			Host:    "host-review.googlesource.com",
			Project: "test-project",
			Ref:     "refs/heads/main",
		},
	},
}

// CreateTestData creates test data used for testing QuerySourceVerdicts.
func CreateTestData() []*spanner.Mutation {
	// Helper to create a test result.
	createResult := func(position int64, rootInvID, invID, resultID string, status pb.TestResult_Status, partitionTime time.Time, subRealm string, changelists []testresults.Changelist, isDirty bool) *TestResult {
		return &TestResult{
			Project:     "test-project",
			TestID:      "test-id",
			VariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
			Sources: testresults.Sources{
				RefHash:     pbutil.SourceRefHash(TestRef),
				Position:    position,
				Changelists: changelists,
				IsDirty:     isDirty,
			},
			RootInvocationID: RootInvocationID{Value: rootInvID, IsLegacy: strings.HasPrefix(rootInvID, "inv-")},
			WorkUnitID:       WorkUnitID{Value: invID, IsLegacy: strings.HasPrefix(rootInvID, "inv-")},
			ResultID:         resultID,
			PartitionTime:    partitionTime,
			SubRealm:         subRealm,
			Status:           pb.TestResultStatus_TEST_RESULT_STATUS_UNSPECIFIED, // Legacy status, ignored by query?
			StatusV2:         status,
		}
	}

	cls := []testresults.Changelist{
		{
			Host:      "host-review.googlesource.com",
			Change:    123,
			Patchset:  1,
			OwnerKind: pb.ChangelistOwnerKind_HUMAN,
		},
	}

	results := []*TestResult{
		// Position 100: 1 pass, 1 fail (Submitted) -> Flaky
		createResult(100, "root-1", "wu-1", "res-1", pb.TestResult_PASSED, time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC), "testrealm", nil, false),
		createResult(100, "root-1", "wu-1", "res-2", pb.TestResult_FAILED, time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC), "testrealm", nil, false),

		// Position 99: 1 pass (Submitted) -> Passed
		createResult(99, "root-2", "wu-2", "res-1", pb.TestResult_PASSED, time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC), "testrealm", nil, false),

		// Position 98: 1 pass (Submitted) -> Passed
		// Legacy export root invocation/parent invocation.
		createResult(98, "inv-3", "inv-3", "res-1", pb.TestResult_PASSED, time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC), "testrealm", nil, false),

		// Position 97: Skipped (Submitted) -> Skipped
		createResult(97, "root-4", "wu-4", "res-1", pb.TestResult_SKIPPED, time.Date(2025, 1, 1, 4, 0, 0, 0, time.UTC), "testrealm", nil, false),

		// Position 96: Execution Errored (Submitted) -> Execution Errored
		createResult(96, "root-5", "wu-5", "res-1", pb.TestResult_EXECUTION_ERRORED, time.Date(2025, 1, 1, 5, 0, 0, 0, time.UTC), "testrealm", nil, false),

		// Position 95: Precluded (Submitted) -> Precluded
		createResult(95, "root-6", "wu-6", "res-1", pb.TestResult_PRECLUDED, time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC), "testrealm", nil, false),

		// Position 94: Presubmit only (Passed) -> Status: Unspecified, Approx Status: Passed
		createResult(94, "root-7", "wu-7", "res-1", pb.TestResult_PASSED, time.Date(2025, 1, 1, 7, 0, 0, 0, time.UTC), "testrealm", cls, false),

		// Position 93: Presubmit (Failed) + Post-submit (Passed) -> Status: Passed, Approx Status: Flaky
		createResult(93, "root-8a", "wu-8a", "res-1", pb.TestResult_FAILED, time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC), "testrealm", cls, false),
		createResult(93, "root-8b", "wu-8b", "res-1", pb.TestResult_PASSED, time.Date(2025, 1, 1, 8, 1, 0, 0, time.UTC), "testrealm", nil, false),

		// Position 92: Dirty sources (Passed) -> Status: Unspecified, Approx Status: Unspecified
		createResult(92, "root-9", "wu-9", "res-1", pb.TestResult_PASSED, time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC), "testrealm", nil, true),

		// Position 91: Different Realm; should be ignored if filtering by "testrealm"
		createResult(91, "root-10", "wu-10", "res-1", pb.TestResult_PASSED, time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), "otherrealm", nil, false),
	}

	var ms []*spanner.Mutation
	for _, r := range results {
		ms = append(ms, r.SaveUnverified())
	}
	return ms
}

// FilterSourceVerdicts filters the given source verdicts using the given filter.
func FilterSourceVerdicts(verdicts []SourceVerdictV2, filter func(v SourceVerdictV2) bool) []SourceVerdictV2 {
	var res []SourceVerdictV2
	for _, v := range verdicts {
		if filter(v) {
			res = append(res, v)
		}
	}
	return res
}

// ExpectedSourceVerdicts returns the expected source verdicts for the test data.
func ExpectedSourceVerdicts() []SourceVerdictV2 {
	cls := []testresults.Changelist{
		{
			Host:      "host-review.googlesource.com",
			Change:    123,
			Patchset:  1,
			OwnerKind: pb.ChangelistOwnerKind_HUMAN,
		},
	}

	return []SourceVerdictV2{
		{
			Position:          100,
			Status:            pb.TestVerdict_FLAKY,
			ApproximateStatus: pb.TestVerdict_FLAKY,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "root-1",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_FLAKY,
					Changelists:      []testresults.Changelist{},
				},
			},
		},
		{
			Position:          99,
			Status:            pb.TestVerdict_PASSED,
			ApproximateStatus: pb.TestVerdict_PASSED,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "root-2",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_PASSED,
					Changelists:      []testresults.Changelist{},
				},
			},
		},
		{
			Position:          98,
			Status:            pb.TestVerdict_PASSED,
			ApproximateStatus: pb.TestVerdict_PASSED,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "",
					InvocationID:     "inv-3",
					PartitionTime:    time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_PASSED,
					Changelists:      []testresults.Changelist{},
				},
			},
		},
		{
			Position:          97,
			Status:            pb.TestVerdict_SKIPPED,
			ApproximateStatus: pb.TestVerdict_SKIPPED,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "root-4",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 4, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_SKIPPED,
					Changelists:      []testresults.Changelist{},
				},
			},
		},
		{
			Position:          96,
			Status:            pb.TestVerdict_EXECUTION_ERRORED,
			ApproximateStatus: pb.TestVerdict_EXECUTION_ERRORED,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "root-5",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 5, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_EXECUTION_ERRORED,
					Changelists:      []testresults.Changelist{},
				},
			},
		},
		{
			Position:          95,
			Status:            pb.TestVerdict_PRECLUDED,
			ApproximateStatus: pb.TestVerdict_PRECLUDED,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "root-6",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_PRECLUDED,
					Changelists:      []testresults.Changelist{},
				},
			},
		},
		{
			Position:          94,
			Status:            pb.TestVerdict_STATUS_UNSPECIFIED,
			ApproximateStatus: pb.TestVerdict_PASSED,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "root-7",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 7, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_PASSED,
					Changelists:      cls,
				},
			},
		},
		{
			Position:          93,
			Status:            pb.TestVerdict_PASSED,
			ApproximateStatus: pb.TestVerdict_FLAKY,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "root-8b",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 8, 1, 0, 0, time.UTC),
					Status:           pb.TestVerdict_PASSED,
					Changelists:      []testresults.Changelist{},
				},
				{
					RootInvocationID: "root-8a",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_FAILED,
					Changelists:      cls,
				},
			},
		},
		{
			Position:          92,
			Status:            pb.TestVerdict_STATUS_UNSPECIFIED,
			ApproximateStatus: pb.TestVerdict_STATUS_UNSPECIFIED,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "root-9",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_PASSED,
					Changelists:      []testresults.Changelist{},
					HasDirtySources:  true,
				},
			},
		},
		{
			Position:          91,
			Status:            pb.TestVerdict_PASSED,
			ApproximateStatus: pb.TestVerdict_PASSED,
			InvocationVerdicts: []InvocationTestVerdictV2{
				{
					RootInvocationID: "root-10",
					InvocationID:     "",
					PartitionTime:    time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
					Status:           pb.TestVerdict_PASSED,
					Changelists:      []testresults.Changelist{},
				},
			},
		},
	}
}

// FilterSourceVerdictsProtos filters the given source verdicts using the given filter.
func FilterSourceVerdictsProtos(verdicts []*pb.SourceVerdict, filter func(v *pb.SourceVerdict) bool) []*pb.SourceVerdict {
	var res []*pb.SourceVerdict
	for _, v := range verdicts {
		if filter(v) {
			res = append(res, v)
		}
	}
	return res
}

// ExpectedSourceVerdictsProto returns the expected source verdicts for the test data
// in protocol buffer format.
func ExpectedSourceVerdictsProto() []*pb.SourceVerdict {
	cls := []*pb.Changelist{
		{
			Host:      "host-review.googlesource.com",
			Change:    123,
			Patchset:  1,
			OwnerKind: pb.ChangelistOwnerKind_HUMAN,
		},
	}

	return []*pb.SourceVerdict{
		{
			Position:          100,
			Status:            pb.TestVerdict_FLAKY,
			ApproximateStatus: pb.TestVerdict_FLAKY,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "rootInvocations/root-1",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_FLAKY,
				},
			},
		},
		{
			Position:          99,
			Status:            pb.TestVerdict_PASSED,
			ApproximateStatus: pb.TestVerdict_PASSED,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "rootInvocations/root-2",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_PASSED,
				},
			},
		},
		{
			Position:          98,
			Status:            pb.TestVerdict_PASSED,
			ApproximateStatus: pb.TestVerdict_PASSED,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "",
					Invocation:     "invocations/inv-3",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_PASSED,
				},
			},
		},
		{
			Position:          97,
			Status:            pb.TestVerdict_SKIPPED,
			ApproximateStatus: pb.TestVerdict_SKIPPED,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "rootInvocations/root-4",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 4, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_SKIPPED,
				},
			},
		},
		{
			Position:          96,
			Status:            pb.TestVerdict_EXECUTION_ERRORED,
			ApproximateStatus: pb.TestVerdict_EXECUTION_ERRORED,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "rootInvocations/root-5",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 5, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_EXECUTION_ERRORED,
				},
			},
		},
		{
			Position:          95,
			Status:            pb.TestVerdict_PRECLUDED,
			ApproximateStatus: pb.TestVerdict_PRECLUDED,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "rootInvocations/root-6",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_PRECLUDED,
				},
			},
		},
		{
			Position:          94,
			Status:            pb.TestVerdict_STATUS_UNSPECIFIED,
			ApproximateStatus: pb.TestVerdict_PASSED,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "rootInvocations/root-7",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 7, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_PASSED,
					Changelists:    cls,
				},
			},
		},
		{
			Position:          93,
			Status:            pb.TestVerdict_PASSED,
			ApproximateStatus: pb.TestVerdict_FLAKY,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "rootInvocations/root-8b",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 8, 1, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_PASSED,
				},
				{
					RootInvocation: "rootInvocations/root-8a",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_FAILED,
					Changelists:    cls,
				},
			},
		},
		{
			Position:          92,
			Status:            pb.TestVerdict_STATUS_UNSPECIFIED,
			ApproximateStatus: pb.TestVerdict_STATUS_UNSPECIFIED,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "rootInvocations/root-9",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_PASSED,
					IsSourcesDirty: true,
				},
			},
		},
		{
			Position:          91,
			Status:            pb.TestVerdict_PASSED,
			ApproximateStatus: pb.TestVerdict_PASSED,
			InvocationVerdicts: []*pb.SourceVerdict_InvocationTestVerdict{
				{
					RootInvocation: "rootInvocations/root-10",
					Invocation:     "",
					PartitionTime:  pbutil.MustTimestampProto(time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)),
					Status:         pb.TestVerdict_PASSED,
				},
			},
		},
	}
}
