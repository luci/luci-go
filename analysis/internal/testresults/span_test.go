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
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestReadTestHistory(t *testing.T) {
	Convey("ReadTestHistory", t, func() {
		ctx := testutil.SpannerTestContext(t)

		referenceTime := time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)
		err := createTestHistoryTestData(ctx, referenceTime)
		So(err, ShouldBeNil)

		opts := ReadTestHistoryOptions{
			Project:   "project",
			TestID:    "test_id",
			SubRealms: []string{"realm", "realm2"},
		}

		expectedChangelists := []*pb.Changelist{
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
		}

		day := 24 * time.Hour
		expectedTestVerdicts := []*pb.TestVerdict{
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant1),
				InvocationId:      "inv1",
				Status:            pb.TestVerdictStatus_EXPECTED,
				PartitionTime:     timestamppb.New(referenceTime.Add(-1 * time.Hour)),
				PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant4),
				InvocationId:      "inv3",
				Status:            pb.TestVerdictStatus_EXPECTED,
				PartitionTime:     timestamppb.New(referenceTime.Add(-1 * time.Hour)),
				PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
				Changelists:       expectedChangelists,
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant1),
				InvocationId:      "inv2",
				Status:            pb.TestVerdictStatus_EXONERATED,
				PartitionTime:     timestamppb.New(referenceTime.Add(-12 * time.Hour)),
				PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant2),
				InvocationId:      "inv1",
				Status:            pb.TestVerdictStatus_FLAKY,
				PartitionTime:     timestamppb.New(referenceTime.Add(-24 * time.Hour)),
				PassedAvgDuration: nil,
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant1),
				InvocationId:      "inv1",
				Status:            pb.TestVerdictStatus_UNEXPECTED,
				PartitionTime:     timestamppb.New(referenceTime.Add(-day - 1*time.Hour)),
				PassedAvgDuration: durationpb.New(33333 * time.Microsecond),
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant1),
				InvocationId:      "inv2",
				Status:            pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED,
				PartitionTime:     timestamppb.New(referenceTime.Add(-day - 12*time.Hour)),
				PassedAvgDuration: nil,
				Changelists:       expectedChangelists,
			}, {
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant2),
				InvocationId:      "inv1",
				Status:            pb.TestVerdictStatus_EXPECTED,
				PartitionTime:     timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
				PassedAvgDuration: nil,
				Changelists:       expectedChangelists,
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant3),
				InvocationId:      "inv1",
				Status:            pb.TestVerdictStatus_EXONERATED,
				PartitionTime:     timestamppb.New(referenceTime.Add(-2*day - 3*time.Hour)),
				PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
				Changelists:       expectedChangelists,
			},
		}

		Convey("baseline", func() {
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, expectedTestVerdicts)
		})
		Convey("with legacy test results data", func() {
			// This test case can be deleted from March 2023. This should be
			// combined with an update to make ChangelistOwnerKinds NOT NULL.
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				stmt := spanner.NewStatement("UPDATE TestResults SET ChangelistOwnerKinds = NULL WHERE TRUE")
				_, err := span.Update(ctx, stmt)
				return err
			})
			So(err, ShouldBeNil)

			for _, v := range expectedTestVerdicts {
				for _, cl := range v.Changelists {
					cl.OwnerKind = pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED
				}
			}

			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, expectedTestVerdicts)
		})
		Convey("pagination works", func() {
			opts.PageSize = 5
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldNotBeEmpty)
			So(verdicts, ShouldResembleProto, expectedTestVerdicts[:5])

			opts.PageToken = nextPageToken
			verdicts, nextPageToken, err = ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, expectedTestVerdicts[5:])
		})

		Convey("with partition_time_range", func() {
			opts.TimeRange = &pb.TimeRange{
				// Inclusive.
				Earliest: timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
				// Exclusive.
				Latest: timestamppb.New(referenceTime.Add(-24 * time.Hour)),
			}
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.TestVerdict{
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_UNEXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 1*time.Hour)),
					PassedAvgDuration: durationpb.New(33333 * time.Microsecond),
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv2",
					Status:            pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 12*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_EXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
			})
		})

		Convey("with contains variant_predicate", func() {
			Convey("with single key-value pair", func() {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2"),
					},
				}
				verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
				So(err, ShouldBeNil)
				So(nextPageToken, ShouldBeEmpty)
				So(verdicts, ShouldResembleProto, []*pb.TestVerdict{
					{
						TestId:            "test_id",
						VariantHash:       pbutil.VariantHash(testVariant2),
						InvocationId:      "inv1",
						Status:            pb.TestVerdictStatus_FLAKY,
						PartitionTime:     timestamppb.New(referenceTime.Add(-24 * time.Hour)),
						PassedAvgDuration: nil,
					},
					{
						TestId:            "test_id",
						VariantHash:       pbutil.VariantHash(testVariant2),
						InvocationId:      "inv1",
						Status:            pb.TestVerdictStatus_EXPECTED,
						PartitionTime:     timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
						PassedAvgDuration: nil,
						Changelists:       expectedChangelists,
					},
					{
						TestId:            "test_id",
						VariantHash:       pbutil.VariantHash(testVariant3),
						InvocationId:      "inv1",
						Status:            pb.TestVerdictStatus_EXONERATED,
						PartitionTime:     timestamppb.New(referenceTime.Add(-2*day - 3*time.Hour)),
						PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
						Changelists:       expectedChangelists,
					},
				})
			})

			Convey("with multiple key-value pairs", func() {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2", "key2", "val2"),
					},
				}
				verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
				So(err, ShouldBeNil)
				So(nextPageToken, ShouldBeEmpty)
				So(verdicts, ShouldResembleProto, []*pb.TestVerdict{
					{
						TestId:            "test_id",
						VariantHash:       pbutil.VariantHash(testVariant3),
						InvocationId:      "inv1",
						Status:            pb.TestVerdictStatus_EXONERATED,
						PartitionTime:     timestamppb.New(referenceTime.Add(-2*day - 3*time.Hour)),
						PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
						Changelists:       expectedChangelists,
					},
				})
			})
		})

		Convey("with equals variant_predicate", func() {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{
					Equals: testVariant2,
				},
			}
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.TestVerdict{
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_FLAKY,
					PartitionTime:     timestamppb.New(referenceTime.Add(-24 * time.Hour)),
					PassedAvgDuration: nil,
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_EXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
			})
		})

		Convey("with hash_equals variant_predicate", func() {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: pbutil.VariantHash(testVariant2),
				},
			}
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.TestVerdict{
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_FLAKY,
					PartitionTime:     timestamppb.New(referenceTime.Add(-24 * time.Hour)),
					PassedAvgDuration: nil,
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_EXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
			})
		})

		Convey("with submitted_filter", func() {
			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_UNSUBMITTED
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.TestVerdict{
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant4),
					InvocationId:      "inv3",
					Status:            pb.TestVerdictStatus_EXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * time.Hour)),
					PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
					Changelists:       expectedChangelists,
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv2",
					Status:            pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 12*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_EXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant3),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_EXONERATED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-2*day - 3*time.Hour)),
					PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
					Changelists:       expectedChangelists,
				},
			})

			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_SUBMITTED
			verdicts, nextPageToken, err = ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.TestVerdict{
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_EXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * time.Hour)),
					PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv2",
					Status:            pb.TestVerdictStatus_EXONERATED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-12 * time.Hour)),
					PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_FLAKY,
					PartitionTime:     timestamppb.New(referenceTime.Add(-24 * time.Hour)),
					PassedAvgDuration: nil,
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_UNEXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 1*time.Hour)),
					PassedAvgDuration: durationpb.New(33333 * time.Microsecond),
				},
			})
		})

		Convey("with bisection filter", func() {
			opts.ExcludeBisectionResults = true
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.TestVerdict{
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_EXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * time.Hour)),
					PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv2",
					Status:            pb.TestVerdictStatus_EXONERATED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-12 * time.Hour)),
					PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_FLAKY,
					PartitionTime:     timestamppb.New(referenceTime.Add(-24 * time.Hour)),
					PassedAvgDuration: nil,
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_UNEXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 1*time.Hour)),
					PassedAvgDuration: durationpb.New(33333 * time.Microsecond),
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant1),
					InvocationId:      "inv2",
					Status:            pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 12*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				}, {
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_EXPECTED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
				{
					TestId:            "test_id",
					VariantHash:       pbutil.VariantHash(testVariant3),
					InvocationId:      "inv1",
					Status:            pb.TestVerdictStatus_EXONERATED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-2*day - 3*time.Hour)),
					PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
					Changelists:       expectedChangelists,
				},
			})
		})
	})
}

func TestReadTestHistoryStats(t *testing.T) {
	Convey("ReadTestHistoryStats", t, func() {
		ctx := testutil.SpannerTestContext(t)

		referenceTime := time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)

		day := 24 * time.Hour

		err := createTestHistoryTestData(ctx, referenceTime)
		So(err, ShouldBeNil)

		opts := ReadTestHistoryOptions{
			Project:   "project",
			TestID:    "test_id",
			SubRealms: []string{"realm", "realm2"},
		}

		expectedGroups := []*pb.QueryTestHistoryStatsResponse_Group{
			{
				PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
				VariantHash:       pbutil.VariantHash(testVariant1),
				ExpectedCount:     1,
				ExoneratedCount:   1,
				PassedAvgDuration: durationpb.New(((22222 + 1234567890123456) / 2) * time.Microsecond),
			},
			{
				PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
				VariantHash:       pbutil.VariantHash(testVariant4),
				ExpectedCount:     1,
				PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
			},
			{
				PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
				VariantHash:       pbutil.VariantHash(testVariant2),
				FlakyCount:        1,
				PassedAvgDuration: nil,
			},
			{
				PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
				VariantHash:              pbutil.VariantHash(testVariant1),
				UnexpectedCount:          1,
				UnexpectedlySkippedCount: 1,
				PassedAvgDuration:        durationpb.New(33333 * time.Microsecond),
			},
			{
				PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
				VariantHash:       pbutil.VariantHash(testVariant2),
				ExpectedCount:     1,
				PassedAvgDuration: nil,
			},
			{
				PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
				VariantHash:       pbutil.VariantHash(testVariant3),
				ExoneratedCount:   1,
				PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
			},
		}

		Convey("baseline", func() {
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, expectedGroups)
		})
		Convey("pagination works", func() {
			opts.PageSize = 4
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldNotBeEmpty)
			So(verdicts, ShouldResembleProto, expectedGroups[:4])

			opts.PageToken = nextPageToken
			verdicts, nextPageToken, err = ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, expectedGroups[4:])
		})
		Convey("with legacy test results data", func() {
			// This test case can be deleted from March 2023. This should be
			// combined with an update to make ChangelistOwnerKinds NOT NULL.
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				stmt := spanner.NewStatement("UPDATE TestResults SET ChangelistOwnerKinds = NULL WHERE TRUE")
				_, err := span.Update(ctx, stmt)
				return err
			})
			So(err, ShouldBeNil)

			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, expectedGroups)
		})

		Convey("with partition_time_range", func() {
			Convey("day boundaries", func() {
				opts.TimeRange = &pb.TimeRange{
					// Inclusive.
					Earliest: timestamppb.New(referenceTime.Add(-2 * day)),
					// Exclusive.
					Latest: timestamppb.New(referenceTime.Add(-1 * day)),
				}
				verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
				So(err, ShouldBeNil)
				So(nextPageToken, ShouldBeEmpty)
				So(verdicts, ShouldResembleProto, []*pb.QueryTestHistoryStatsResponse_Group{
					{
						PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:              pbutil.VariantHash(testVariant1),
						UnexpectedCount:          1,
						UnexpectedlySkippedCount: 1,
						PassedAvgDuration:        durationpb.New(33333 * time.Microsecond),
					},
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:       pbutil.VariantHash(testVariant2),
						ExpectedCount:     1,
						PassedAvgDuration: nil,
					},
				})
			})
			Convey("part-day boundaries", func() {
				opts.TimeRange = &pb.TimeRange{
					// Inclusive.
					Earliest: timestamppb.New(referenceTime.Add(-2*day - 3*time.Hour)),
					// Exclusive.
					Latest: timestamppb.New(referenceTime.Add(-1*day - 1*time.Hour)),
				}
				verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
				So(err, ShouldBeNil)
				So(nextPageToken, ShouldBeEmpty)
				So(verdicts, ShouldResembleProto, []*pb.QueryTestHistoryStatsResponse_Group{
					{
						PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:              pbutil.VariantHash(testVariant1),
						UnexpectedlySkippedCount: 1,
						PassedAvgDuration:        nil,
					},
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:       pbutil.VariantHash(testVariant2),
						ExpectedCount:     1,
						PassedAvgDuration: nil,
					},
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
						VariantHash:       pbutil.VariantHash(testVariant3),
						ExoneratedCount:   1,
						PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
					},
				})
			})
		})

		Convey("with contains variant_predicate", func() {
			Convey("with single key-value pair", func() {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2"),
					},
				}
				verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
				So(err, ShouldBeNil)
				So(nextPageToken, ShouldBeEmpty)
				So(verdicts, ShouldResembleProto, []*pb.QueryTestHistoryStatsResponse_Group{
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
						VariantHash:       pbutil.VariantHash(testVariant2),
						FlakyCount:        1,
						PassedAvgDuration: nil,
					},
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:       pbutil.VariantHash(testVariant2),
						ExpectedCount:     1,
						PassedAvgDuration: nil,
					},
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
						VariantHash:       pbutil.VariantHash(testVariant3),
						ExoneratedCount:   1,
						PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
					},
				})
			})

			Convey("with multiple key-value pairs", func() {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2", "key2", "val2"),
					},
				}
				verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
				So(err, ShouldBeNil)
				So(nextPageToken, ShouldBeEmpty)
				So(verdicts, ShouldResembleProto, []*pb.QueryTestHistoryStatsResponse_Group{
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
						VariantHash:       pbutil.VariantHash(testVariant3),
						ExoneratedCount:   1,
						PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
					},
				})
			})
		})

		Convey("with equals variant_predicate", func() {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{
					Equals: testVariant2,
				},
			}
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					FlakyCount:        1,
					PassedAvgDuration: nil,
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					ExpectedCount:     1,
					PassedAvgDuration: nil,
				},
			})
		})

		Convey("with hash_equals variant_predicate", func() {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: pbutil.VariantHash(testVariant2),
				},
			}
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					FlakyCount:        1,
					PassedAvgDuration: nil,
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					ExpectedCount:     1,
					PassedAvgDuration: nil,
				},
			})
		})

		Convey("with empty hash_equals variant_predicate", func() {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: "",
				},
			}
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldBeEmpty)
		})

		Convey("with submitted_filter", func() {
			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_UNSUBMITTED
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant4),
					ExpectedCount:     1,
					PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
				},
				{
					PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:              pbutil.VariantHash(testVariant1),
					UnexpectedlySkippedCount: 1,
					PassedAvgDuration:        nil,
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					ExpectedCount:     1,
					PassedAvgDuration: nil,
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
					VariantHash:       pbutil.VariantHash(testVariant3),
					ExoneratedCount:   1,
					PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
				},
			})

			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_SUBMITTED
			verdicts, nextPageToken, err = ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant1),
					ExpectedCount:     1,
					ExoneratedCount:   1,
					PassedAvgDuration: durationpb.New(((22222 + 1234567890123456) / 2) * time.Microsecond),
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					FlakyCount:        1,
					PassedAvgDuration: nil,
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant1),
					UnexpectedCount:   1,
					PassedAvgDuration: durationpb.New(33333 * time.Microsecond),
				},
			})
		})

		Convey("with bisection filter", func() {
			opts.ExcludeBisectionResults = true
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(verdicts, ShouldResembleProto, []*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant1),
					ExpectedCount:     1,
					ExoneratedCount:   1,
					PassedAvgDuration: durationpb.New(((22222 + 1234567890123456) / 2) * time.Microsecond),
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					FlakyCount:        1,
					PassedAvgDuration: nil,
				},
				{
					PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:              pbutil.VariantHash(testVariant1),
					UnexpectedCount:          1,
					UnexpectedlySkippedCount: 1,
					PassedAvgDuration:        durationpb.New(33333 * time.Microsecond),
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					ExpectedCount:     1,
					PassedAvgDuration: nil,
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
					VariantHash:       pbutil.VariantHash(testVariant3),
					ExoneratedCount:   1,
					PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
				},
			})
		})
	})
}

func TestReadVariants(t *testing.T) {
	Convey("ReadVariants", t, func() {
		ctx := testutil.SpannerTestContext(t)

		var1 := pbutil.Variant("key1", "val1", "key2", "val1")
		var2 := pbutil.Variant("key1", "val2", "key2", "val1")
		var3 := pbutil.Variant("key1", "val2", "key2", "val2")
		var4 := pbutil.Variant("key1", "val1", "key2", "val2")

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

			insertTVR("realm1", var1)
			insertTVR("realm1", var2)

			insertTVR("realm2", var2)
			insertTVR("realm2", var3)

			insertTVR("realm3", var4)

			return nil
		})
		So(err, ShouldBeNil)

		Convey("pagination works", func() {
			opts := ReadVariantsOptions{PageSize: 3, SubRealms: []string{"realm1", "realm2", "realm3"}}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldNotBeEmpty)
			So(variants, ShouldResembleProto, []*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var1),
					Variant:     var1,
				},
				{
					VariantHash: pbutil.VariantHash(var3),
					Variant:     var3,
				},
				{
					VariantHash: pbutil.VariantHash(var4),
					Variant:     var4,
				},
			})

			opts.PageToken = nextPageToken
			variants, nextPageToken, err = ReadVariants(span.Single(ctx), "project", "test_id", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(variants, ShouldResembleProto, []*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			})
		})

		Convey("multi-realm works", func() {
			opts := ReadVariantsOptions{SubRealms: []string{"realm1", "realm2"}}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(variants, ShouldResembleProto, []*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var1),
					Variant:     var1,
				},
				{
					VariantHash: pbutil.VariantHash(var3),
					Variant:     var3,
				},
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			})
		})

		Convey("single-realm works", func() {
			opts := ReadVariantsOptions{SubRealms: []string{"realm2"}}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(variants, ShouldResembleProto, []*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var3),
					Variant:     var3,
				},
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			})
		})

		Convey("with contains variant predicate", func() {
			Convey("with single key-value pair", func() {
				opts := ReadVariantsOptions{
					SubRealms: []string{"realm1", "realm2"},
					VariantPredicate: &pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Contains{
							Contains: pbutil.Variant("key1", "val2"),
						},
					},
				}
				variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
				So(err, ShouldBeNil)
				So(nextPageToken, ShouldBeEmpty)
				So(variants, ShouldResembleProto, []*pb.QueryVariantsResponse_VariantInfo{
					{
						VariantHash: pbutil.VariantHash(var3),
						Variant:     var3,
					},
					{
						VariantHash: pbutil.VariantHash(var2),
						Variant:     var2,
					},
				})
			})

			Convey("with multiple key-value pairs", func() {
				opts := ReadVariantsOptions{
					SubRealms: []string{"realm1", "realm2"},
					VariantPredicate: &pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Contains{
							Contains: pbutil.Variant("key1", "val2", "key2", "val2"),
						},
					},
				}
				variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
				So(err, ShouldBeNil)
				So(nextPageToken, ShouldBeEmpty)
				So(variants, ShouldResembleProto, []*pb.QueryVariantsResponse_VariantInfo{
					{
						VariantHash: pbutil.VariantHash(var3),
						Variant:     var3,
					},
				})
			})
		})

		Convey("with equals variant predicate", func() {
			opts := ReadVariantsOptions{
				SubRealms: []string{"realm1", "realm2"},
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{
						Equals: var2,
					},
				},
			}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(variants, ShouldResembleProto, []*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			})
		})

		Convey("with hash_equals variant predicate", func() {
			opts := ReadVariantsOptions{
				SubRealms: []string{"realm2"},
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{
						HashEquals: pbutil.VariantHash(var2),
					},
				},
			}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(variants, ShouldResembleProto, []*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			})
		})
	})
}

func TestQueryTests(t *testing.T) {
	Convey("QueryTests", t, func() {
		ctx := testutil.SpannerTestContext(t)

		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			insertTest := func(subRealm string, testID string) {
				span.BufferWrite(ctx, (&TestRealm{
					Project:  "project",
					TestID:   testID,
					SubRealm: subRealm,
				}).SaveUnverified())
			}

			insertTest("realm1", "test-id00")
			insertTest("realm2", "test-id01")
			insertTest("realm3", "test-id02")

			insertTest("realm1", "test-id10")
			insertTest("realm2", "test-id11")
			insertTest("realm3", "test-id12")

			insertTest("realm1", "test-id20")
			insertTest("realm2", "test-id21")
			insertTest("realm3", "test-id22")

			insertTest("realm1", "special%_characters")
			insertTest("realm1", "specialxxcharacters")

			return nil
		})
		So(err, ShouldBeNil)

		Convey("pagination works", func() {
			opts := QueryTestsOptions{PageSize: 2, SubRealms: []string{"realm1", "realm2", "realm3"}}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "id1", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldNotBeEmpty)
			So(testIDs, ShouldResemble, []string{
				"test-id10",
				"test-id11",
			})

			opts.PageToken = nextPageToken
			testIDs, nextPageToken, err = QueryTests(span.Single(ctx), "project", "id1", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(testIDs, ShouldResemble, []string{
				"test-id12",
			})
		})

		Convey("multi-realm works", func() {
			opts := QueryTestsOptions{SubRealms: []string{"realm1", "realm2"}}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "test-id", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(testIDs, ShouldResemble, []string{
				"test-id00",
				"test-id01",
				"test-id10",
				"test-id11",
				"test-id20",
				"test-id21",
			})
		})

		Convey("single-realm works", func() {
			opts := QueryTestsOptions{SubRealms: []string{"realm3"}}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "test-id", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(testIDs, ShouldResemble, []string{
				"test-id02",
				"test-id12",
				"test-id22",
			})
		})

		Convey("special character works", func() {
			opts := QueryTestsOptions{SubRealms: []string{"realm1", "realm2", "realm3"}}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "special%_characters", opts)
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldBeEmpty)
			So(testIDs, ShouldResemble, []string{
				"special%_characters",
			})
		})
	})
}
