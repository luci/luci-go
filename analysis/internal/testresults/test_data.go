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

package testresults

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var testVariant1 = pbutil.Variant("key1", "val1", "key2", "val1")
var testVariant2 = pbutil.Variant("key1", "val2", "key2", "val1")
var testVariant3 = pbutil.Variant("key1", "val2", "key2", "val2")
var testVariant4 = pbutil.Variant("key1", "val1", "key2", "val2")

func createTestHistoryTestData(ctx context.Context, referenceTime time.Time) error {
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		insertTVR := func(subRealm string, variant *pb.Variant) {
			span.BufferWrite(ctx, (&TestVariantRealm{
				Project:     "project",
				TestID:      "test_id",
				SubRealm:    subRealm,
				Variant:     variant,
				VariantHash: pbutil.VariantHash(variant),
			}).SaveUnverified())
		}

		insertTVR("realm", testVariant1)
		insertTVR("realm", testVariant2)
		insertTVR("realm", testVariant3)
		insertTVR("realm2", testVariant4)

		insertTV := func(partitionTime time.Time, variant *pb.Variant, invId string, status pb.TestVerdictStatus, hasUnsubmittedChanges bool, isFromBisection bool, avgDuration *time.Duration) {
			baseTestResult := NewTestResult().
				WithProject("project").
				WithTestID("test_id").
				WithVariantHash(pbutil.VariantHash(variant)).
				WithPartitionTime(partitionTime).
				WithIngestedInvocationID(invId).
				WithSubRealm("realm").
				WithStatus(pb.TestResultStatus_PASS).
				WithIsFromBisection(isFromBisection)
			if hasUnsubmittedChanges {
				sources := Sources{
					Changelists: []Changelist{
						{
							Host:      "anothergerrit.gerrit.instance",
							Change:    5471,
							Patchset:  6,
							OwnerKind: pb.ChangelistOwnerKind_HUMAN,
						},
						{
							Host:      "mygerrit-review.googlesource.com",
							Change:    4321,
							Patchset:  5,
							OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
						},
					},
				}
				baseTestResult = baseTestResult.WithSources(sources)
			} else {
				baseTestResult = baseTestResult.WithSources(Sources{})
			}

			trs := NewTestVerdict().
				WithBaseTestResult(baseTestResult.Build()).
				WithStatus(status).
				WithPassedAvgDuration(avgDuration).
				Build()
			for _, tr := range trs {
				span.BufferWrite(ctx, tr.SaveUnverified())
			}
		}

		day := 24 * time.Hour
		insertTV(referenceTime.Add(-1*time.Hour), testVariant1, "inv1", pb.TestVerdictStatus_EXPECTED, false, false, newDuration(22222*time.Microsecond))
		insertTV(referenceTime.Add(-12*time.Hour), testVariant1, "inv2", pb.TestVerdictStatus_EXONERATED, false, false, newDuration(1234567890123456*time.Microsecond))
		insertTV(referenceTime.Add(-24*time.Hour), testVariant2, "inv1", pb.TestVerdictStatus_FLAKY, false, false, nil)

		insertTV(referenceTime.Add(-day-1*time.Hour), testVariant1, "inv1", pb.TestVerdictStatus_UNEXPECTED, false, false, newDuration(33333*time.Microsecond))
		insertTV(referenceTime.Add(-day-12*time.Hour), testVariant1, "inv2", pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED, true, false, nil)
		insertTV(referenceTime.Add(-day-24*time.Hour), testVariant2, "inv1", pb.TestVerdictStatus_EXPECTED, true, false, nil)

		insertTV(referenceTime.Add(-2*day-3*time.Hour), testVariant3, "inv1", pb.TestVerdictStatus_EXONERATED, true, false, newDuration(88888*time.Microsecond))
		insertTV(referenceTime.Add(-1*time.Hour), testVariant4, "inv3", pb.TestVerdictStatus_EXPECTED, true, true, newDuration(22222*time.Microsecond))

		return nil
	})
	return err
}

func newDuration(value time.Duration) *time.Duration {
	d := new(time.Duration)
	*d = value
	return d
}

// June 17th, 2022 is a Friday. The preceding 5 * 24 weekday hour
// are as follows:
//
//	Inclusive              - Exclusive
//
// Interval 0: (-1 day) Thursday 8am  - (now)    Friday 8am
// Interval 1: (-2 day) Wednesday 8am - (-1 day) Thursday 8am
// Interval 2: (-3 day) Tuesday 8am   - (-2 day) Wednesday 8am
// Interval 3: (-4 day) Monday 8am    - (-3 day) Tuesday 8am
// Interval 4: (-7 day) Friday 8am    - (-4 day) Monday 8am
var referenceTime = time.Date(2022, time.June, 17, 8, 0, 0, 0, time.UTC)

// CreateQueryFailureRateTestData creates test data in Spanner for testing
// QueryFailureRate.
func CreateQueryFailureRateTestData(ctx context.Context) error {
	var1 := pbutil.Variant("key1", "val1", "key2", "val1")
	var2 := pbutil.Variant("key1", "val2", "key2", "val1")
	var3 := pbutil.Variant("key1", "val2", "key2", "val2")

	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		insertTV := func(partitionTime time.Time, variant *pb.Variant, invId string, runStatuses []RunStatus, clOwnerKind pb.ChangelistOwnerKind, changeListNumber ...int64) {
			baseTestResult := NewTestResult().
				WithProject("project").
				WithTestID("test_id").
				WithVariantHash(pbutil.VariantHash(variant)).
				WithPartitionTime(partitionTime).
				WithIngestedInvocationID(invId).
				WithSubRealm("realm").
				WithStatus(pb.TestResultStatus_PASS)

			var changelists []Changelist
			for _, clNum := range changeListNumber {
				changelists = append(changelists, Changelist{
					Host:      "mygerrit-review.googlesource.com",
					Change:    clNum,
					Patchset:  5,
					OwnerKind: clOwnerKind,
				})
			}
			sources := Sources{
				Changelists: changelists,
			}
			baseTestResult = baseTestResult.WithSources(sources)

			trs := NewTestVerdict().
				WithBaseTestResult(baseTestResult.Build()).
				WithRunStatus(runStatuses...).
				Build()
			for _, tr := range trs {
				span.BufferWrite(ctx, tr.SaveUnverified())
			}
		}

		// pass, fail is shorthand here for expected and unexpected run,
		// where for the purposes of this RPC, a flaky run counts as
		// "expected" (as it has at least one expected result).
		passFail := []RunStatus{Flaky, Unexpected}
		failPass := []RunStatus{Unexpected, Flaky}
		pass := []RunStatus{Flaky}
		fail := []RunStatus{Unexpected}
		failFail := []RunStatus{Unexpected, Unexpected}

		day := 24 * time.Hour

		automationOwner := pb.ChangelistOwnerKind_AUTOMATION
		humanOwner := pb.ChangelistOwnerKind_HUMAN
		unspecifiedOwner := pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED

		insertTV(referenceTime.Add(-6*day), var1, "inv1", failPass, unspecifiedOwner)
		// duplicate-cl result should not be used, inv3 result should be
		// used instead (as only one verdict per changelist is used, and
		// inv3 is more recent).
		insertTV(referenceTime.Add(-4*day), var1, "duplicate-cl", failPass, humanOwner, 1)
		// duplicate-cl2 result should not be used, inv3 result should be used instead
		// (as only one verdict per changelist is used, and inv3 is flaky
		// and this is not).
		insertTV(referenceTime.Add(-1*time.Hour), var1, "duplicate-cl2", pass, humanOwner, 1)

		insertTV(referenceTime.Add(-4*day), var1, "inv2", pass, unspecifiedOwner, 2)
		insertTV(referenceTime.Add(-2*time.Hour), var1, "inv3", failPass, humanOwner, 1)

		// inv4 should not be used as the CL tested is authored by automation.
		insertTV(referenceTime.Add(-3*day), var1, "inv4", failPass, automationOwner, 11)
		insertTV(referenceTime.Add(-3*day), var1, "inv5", passFail, unspecifiedOwner, 3)
		insertTV(referenceTime.Add(-2*day), var1, "inv6", fail, humanOwner, 4)
		insertTV(referenceTime.Add(-3*day), var1, "inv7", failFail, unspecifiedOwner)
		// should not be used, as tests multiple CLs, and too hard
		// to deduplicate the verdicts.
		insertTV(referenceTime.Add(-2*day), var1, "many-cl", failPass, humanOwner, 1, 3)

		// should not be used, as times  fall outside the queried intervals.
		insertTV(referenceTime.Add(-7*day-time.Microsecond), var1, "too-early", failPass, humanOwner, 5)
		insertTV(referenceTime, var1, "too-late", failPass, humanOwner, 6)

		insertTV(referenceTime.Add(-4*day), var2, "inv1", failPass, humanOwner, 1)
		insertTV(referenceTime.Add(-3*day), var2, "inv2", failPass, humanOwner, 2)

		insertTV(referenceTime.Add(-5*day), var3, "duplicate-cl1", passFail, humanOwner, 1)
		insertTV(referenceTime.Add(-3*day), var3, "duplicate-cl2", failPass, humanOwner, 1)
		insertTV(referenceTime.Add(-1*day), var3, "inv8", failPass, humanOwner, 1)

		return nil
	})
	return err
}

func QueryFailureRateSampleRequest() (project string, asAtTime time.Time, testVariants []*pb.QueryTestVariantFailureRateRequest_TestVariant) {
	var1 := pbutil.Variant("key1", "val1", "key2", "val1")
	var3 := pbutil.Variant("key1", "val2", "key2", "val2")
	testVariants = []*pb.QueryTestVariantFailureRateRequest_TestVariant{
		{
			TestId:  "test_id",
			Variant: var1,
		},
		{
			TestId:  "test_id",
			Variant: var3,
		},
	}
	asAtTime = time.Date(2022, time.June, 17, 8, 0, 0, 0, time.UTC)
	return "project", asAtTime, testVariants
}

// QueryFailureRateSampleResponse returns expected response data from QueryFailureRate
// after being invoked with QueryFailureRateSampleRequest.
// It is assumed test data was setup with CreateQueryFailureRateTestData.
func QueryFailureRateSampleResponse() *pb.QueryTestVariantFailureRateResponse {
	var1 := pbutil.Variant("key1", "val1", "key2", "val1")
	var3 := pbutil.Variant("key1", "val2", "key2", "val2")

	day := 24 * time.Hour

	intervals := []*pb.QueryTestVariantFailureRateResponse_Interval{
		{
			IntervalAge: 1,
			StartTime:   timestamppb.New(referenceTime.Add(-1 * day)),
			EndTime:     timestamppb.New(referenceTime),
		},
		{
			IntervalAge: 2,
			StartTime:   timestamppb.New(referenceTime.Add(-2 * day)),
			EndTime:     timestamppb.New(referenceTime.Add(-1 * day)),
		},
		{
			IntervalAge: 3,
			StartTime:   timestamppb.New(referenceTime.Add(-3 * day)),
			EndTime:     timestamppb.New(referenceTime.Add(-2 * day)),
		},
		{
			IntervalAge: 4,
			StartTime:   timestamppb.New(referenceTime.Add(-4 * day)),
			EndTime:     timestamppb.New(referenceTime.Add(-3 * day)),
		},
		{
			IntervalAge: 5,
			StartTime:   timestamppb.New(referenceTime.Add(-7 * day)),
			EndTime:     timestamppb.New(referenceTime.Add(-4 * day)),
		},
	}

	analysis := []*pb.TestVariantFailureRateAnalysis{
		{
			TestId:  "test_id",
			Variant: var1,
			IntervalStats: []*pb.TestVariantFailureRateAnalysis_IntervalStats{
				{
					IntervalAge:           1,
					TotalRunFlakyVerdicts: 1, // inv3.
				},
				{
					IntervalAge:                2,
					TotalRunUnexpectedVerdicts: 1, // inv6.
				},
				{
					IntervalAge:                3,
					TotalRunFlakyVerdicts:      1, // inv5.
					TotalRunUnexpectedVerdicts: 1, // inv7.
				},
				{
					IntervalAge:              4,
					TotalRunExpectedVerdicts: 1, // inv2.
				},
				{
					IntervalAge:           5,
					TotalRunFlakyVerdicts: 1, //inv1.

				},
			},
			RunFlakyVerdictExamples: []*pb.TestVariantFailureRateAnalysis_VerdictExample{
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-2 * time.Hour)),
					IngestedInvocationId: "inv3",
					Changelists:          expectedPBChangelist(1, pb.ChangelistOwnerKind_HUMAN),
				},
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-3 * day)),
					IngestedInvocationId: "inv5",
					Changelists:          expectedPBChangelist(3, pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED),
				},
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-6 * day)),
					IngestedInvocationId: "inv1",
				},
			},
			// inv4 should not be included as it is a CL authored by automation.
			RecentVerdicts: []*pb.TestVariantFailureRateAnalysis_RecentVerdict{
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-2 * time.Hour)),
					IngestedInvocationId: "inv3",
					Changelists:          expectedPBChangelist(1, pb.ChangelistOwnerKind_HUMAN),
					HasUnexpectedRuns:    true,
				},
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-2 * day)),
					IngestedInvocationId: "inv6",
					Changelists:          expectedPBChangelist(4, pb.ChangelistOwnerKind_HUMAN),
					HasUnexpectedRuns:    true,
				},
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-3 * day)),
					IngestedInvocationId: "inv5",
					Changelists:          expectedPBChangelist(3, pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED),
					HasUnexpectedRuns:    true,
				},
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-3 * day)),
					IngestedInvocationId: "inv7",
					HasUnexpectedRuns:    true,
				},
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-4 * day)),
					IngestedInvocationId: "inv2",
					Changelists:          expectedPBChangelist(2, pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED),
					HasUnexpectedRuns:    false,
				},
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-6 * day)),
					IngestedInvocationId: "inv1",
					HasUnexpectedRuns:    true,
				},
			},
		},
		{
			TestId:  "test_id",
			Variant: var3,
			IntervalStats: []*pb.TestVariantFailureRateAnalysis_IntervalStats{
				{
					IntervalAge:           1,
					TotalRunFlakyVerdicts: 1, // inv8.
				},
				{
					IntervalAge: 2,
				},
				{
					IntervalAge: 3,
				},
				{
					IntervalAge: 4,
				},
				{
					IntervalAge: 5,
				},
			},
			RunFlakyVerdictExamples: []*pb.TestVariantFailureRateAnalysis_VerdictExample{
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-1 * day)),
					IngestedInvocationId: "inv8",
					Changelists:          expectedPBChangelist(1, pb.ChangelistOwnerKind_HUMAN),
				},
			},
			RecentVerdicts: []*pb.TestVariantFailureRateAnalysis_RecentVerdict{
				{
					PartitionTime:        timestamppb.New(referenceTime.Add(-1 * day)),
					IngestedInvocationId: "inv8",
					Changelists:          expectedPBChangelist(1, pb.ChangelistOwnerKind_HUMAN),
					HasUnexpectedRuns:    true,
				},
			},
		},
	}

	return &pb.QueryTestVariantFailureRateResponse{
		Intervals:    intervals,
		TestVariants: analysis,
	}
}

func expectedPBChangelist(change int64, ownerKind pb.ChangelistOwnerKind) []*pb.Changelist {
	return []*pb.Changelist{
		{
			Host:      "mygerrit-review.googlesource.com",
			Change:    change,
			Patchset:  5,
			OwnerKind: ownerKind,
		},
	}
}
