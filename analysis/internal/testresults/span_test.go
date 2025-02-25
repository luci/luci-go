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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestReadTestHistory(t *testing.T) {
	ftt.Run("ReadTestHistory", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		referenceTime := time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)
		err := createTestHistoryTestData(ctx, referenceTime)
		assert.Loosely(t, err, should.BeNil)

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

		t.Run("baseline", func(t *ftt.Test) {
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts))
		})
		t.Run("with legacy test results data", func(t *ftt.Test) {
			// This test case can be deleted from March 2023. This should be
			// combined with an update to make ChangelistOwnerKinds NOT NULL.
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				stmt := spanner.NewStatement("UPDATE TestResults SET ChangelistOwnerKinds = NULL WHERE TRUE")
				_, err := span.Update(ctx, stmt)
				return err
			})
			assert.Loosely(t, err, should.BeNil)

			for _, v := range expectedTestVerdicts {
				for _, cl := range v.Changelists {
					cl.OwnerKind = pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED
				}
			}

			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts))
		})
		t.Run("pagination works", func(t *ftt.Test) {
			opts.PageSize = 5
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.NotBeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts[:5]))

			opts.PageToken = nextPageToken
			verdicts, nextPageToken, err = ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts[5:]))
		})

		t.Run("with partition_time_range", func(t *ftt.Test) {
			opts.TimeRange = &pb.TimeRange{
				// Inclusive.
				Earliest: timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
				// Exclusive.
				Latest: timestamppb.New(referenceTime.Add(-24 * time.Hour)),
			}
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
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
			}))
		})

		t.Run("with contains variant_predicate", func(t *ftt.Test) {
			t.Run("with single key-value pair", func(t *ftt.Test) {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2"),
					},
				}
				verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
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
				}))
			})

			t.Run("with multiple key-value pairs", func(t *ftt.Test) {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2", "key2", "val2"),
					},
				}
				verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
					{
						TestId:            "test_id",
						VariantHash:       pbutil.VariantHash(testVariant3),
						InvocationId:      "inv1",
						Status:            pb.TestVerdictStatus_EXONERATED,
						PartitionTime:     timestamppb.New(referenceTime.Add(-2*day - 3*time.Hour)),
						PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
						Changelists:       expectedChangelists,
					},
				}))
			})
		})

		t.Run("with equals variant_predicate", func(t *ftt.Test) {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{
					Equals: testVariant2,
				},
			}
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
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
			}))
		})

		t.Run("with hash_equals variant_predicate", func(t *ftt.Test) {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: pbutil.VariantHash(testVariant2),
				},
			}
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
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
			}))
		})

		t.Run("with submitted_filter", func(t *ftt.Test) {
			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_UNSUBMITTED
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
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
			}))

			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_SUBMITTED
			verdicts, nextPageToken, err = ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
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
			}))
		})

		t.Run("with bisection filter", func(t *ftt.Test) {
			opts.ExcludeBisectionResults = true
			verdicts, nextPageToken, err := ReadTestHistory(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
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
			}))
		})
	})
}

func TestReadTestHistoryStats(t *testing.T) {
	ftt.Run("ReadTestHistoryStats", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		referenceTime := time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)

		day := 24 * time.Hour

		err := createTestHistoryTestData(ctx, referenceTime)
		assert.Loosely(t, err, should.BeNil)

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

		t.Run("baseline", func(t *ftt.Test) {
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedGroups))
		})
		t.Run("pagination works", func(t *ftt.Test) {
			opts.PageSize = 4
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.NotBeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedGroups[:4]))

			opts.PageToken = nextPageToken
			verdicts, nextPageToken, err = ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedGroups[4:]))
		})
		t.Run("with legacy test results data", func(t *ftt.Test) {
			// This test case can be deleted from March 2023. This should be
			// combined with an update to make ChangelistOwnerKinds NOT NULL.
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				stmt := spanner.NewStatement("UPDATE TestResults SET ChangelistOwnerKinds = NULL WHERE TRUE")
				_, err := span.Update(ctx, stmt)
				return err
			})
			assert.Loosely(t, err, should.BeNil)

			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedGroups))
		})

		t.Run("with partition_time_range", func(t *ftt.Test) {
			t.Run("day boundaries", func(t *ftt.Test) {
				opts.TimeRange = &pb.TimeRange{
					// Inclusive.
					Earliest: timestamppb.New(referenceTime.Add(-2 * day)),
					// Exclusive.
					Latest: timestamppb.New(referenceTime.Add(-1 * day)),
				}
				verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
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
				}))
			})
			t.Run("part-day boundaries", func(t *ftt.Test) {
				opts.TimeRange = &pb.TimeRange{
					// Inclusive.
					Earliest: timestamppb.New(referenceTime.Add(-2*day - 3*time.Hour)),
					// Exclusive.
					Latest: timestamppb.New(referenceTime.Add(-1*day - 1*time.Hour)),
				}
				verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
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
				}))
			})
		})

		t.Run("with contains variant_predicate", func(t *ftt.Test) {
			t.Run("with single key-value pair", func(t *ftt.Test) {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2"),
					},
				}
				verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
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
				}))
			})

			t.Run("with multiple key-value pairs", func(t *ftt.Test) {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2", "key2", "val2"),
					},
				}
				verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
						VariantHash:       pbutil.VariantHash(testVariant3),
						ExoneratedCount:   1,
						PassedAvgDuration: durationpb.New(88888 * time.Microsecond),
					},
				}))
			})
		})

		t.Run("with equals variant_predicate", func(t *ftt.Test) {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{
					Equals: testVariant2,
				},
			}
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
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
			}))
		})

		t.Run("with hash_equals variant_predicate", func(t *ftt.Test) {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: pbutil.VariantHash(testVariant2),
				},
			}
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
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
			}))
		})

		t.Run("with empty hash_equals variant_predicate", func(t *ftt.Test) {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: "",
				},
			}
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.BeEmpty)
		})

		t.Run("with submitted_filter", func(t *ftt.Test) {
			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_UNSUBMITTED
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
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
			}))

			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_SUBMITTED
			verdicts, nextPageToken, err = ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
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
			}))
		})

		t.Run("with bisection filter", func(t *ftt.Test) {
			opts.ExcludeBisectionResults = true
			verdicts, nextPageToken, err := ReadTestHistoryStats(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
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
			}))
		})
	})
}

func TestReadVariants(t *testing.T) {
	ftt.Run("ReadVariants", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		t.Run("pagination works", func(t *ftt.Test) {
			opts := ReadVariantsOptions{PageSize: 3, SubRealms: []string{"realm1", "realm2", "realm3"}}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.NotBeEmpty)
			assert.Loosely(t, variants, should.Match([]*pb.QueryVariantsResponse_VariantInfo{
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
			}))

			opts.PageToken = nextPageToken
			variants, nextPageToken, err = ReadVariants(span.Single(ctx), "project", "test_id", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, variants, should.Match([]*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			}))
		})

		t.Run("multi-realm works", func(t *ftt.Test) {
			opts := ReadVariantsOptions{SubRealms: []string{"realm1", "realm2"}}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, variants, should.Match([]*pb.QueryVariantsResponse_VariantInfo{
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
			}))
		})

		t.Run("single-realm works", func(t *ftt.Test) {
			opts := ReadVariantsOptions{SubRealms: []string{"realm2"}}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, variants, should.Match([]*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var3),
					Variant:     var3,
				},
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			}))
		})

		t.Run("with contains variant predicate", func(t *ftt.Test) {
			t.Run("with single key-value pair", func(t *ftt.Test) {
				opts := ReadVariantsOptions{
					SubRealms: []string{"realm1", "realm2"},
					VariantPredicate: &pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Contains{
							Contains: pbutil.Variant("key1", "val2"),
						},
					},
				}
				variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, variants, should.Match([]*pb.QueryVariantsResponse_VariantInfo{
					{
						VariantHash: pbutil.VariantHash(var3),
						Variant:     var3,
					},
					{
						VariantHash: pbutil.VariantHash(var2),
						Variant:     var2,
					},
				}))
			})

			t.Run("with multiple key-value pairs", func(t *ftt.Test) {
				opts := ReadVariantsOptions{
					SubRealms: []string{"realm1", "realm2"},
					VariantPredicate: &pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Contains{
							Contains: pbutil.Variant("key1", "val2", "key2", "val2"),
						},
					},
				}
				variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, variants, should.Match([]*pb.QueryVariantsResponse_VariantInfo{
					{
						VariantHash: pbutil.VariantHash(var3),
						Variant:     var3,
					},
				}))
			})
		})

		t.Run("with equals variant predicate", func(t *ftt.Test) {
			opts := ReadVariantsOptions{
				SubRealms: []string{"realm1", "realm2"},
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{
						Equals: var2,
					},
				},
			}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, variants, should.Match([]*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			}))
		})

		t.Run("with hash_equals variant predicate", func(t *ftt.Test) {
			opts := ReadVariantsOptions{
				SubRealms: []string{"realm2"},
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{
						HashEquals: pbutil.VariantHash(var2),
					},
				},
			}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), "project", "test_id", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, variants, should.Match([]*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			}))
		})
	})
}

func TestQueryTests(t *testing.T) {
	ftt.Run("QueryTests", t, func(t *ftt.Test) {
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

			insertTest("realm1", "caseSensitive")

			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run("pagination works", func(t *ftt.Test) {
			opts := QueryTestsOptions{PageSize: 2, SubRealms: []string{"realm1", "realm2", "realm3"}}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "id1", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.NotBeEmpty)
			assert.Loosely(t, testIDs, should.Match([]string{
				"test-id10",
				"test-id11",
			}))

			opts.PageToken = nextPageToken
			testIDs, nextPageToken, err = QueryTests(span.Single(ctx), "project", "id1", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, testIDs, should.Match([]string{
				"test-id12",
			}))
		})

		t.Run("multi-realm works", func(t *ftt.Test) {
			opts := QueryTestsOptions{SubRealms: []string{"realm1", "realm2"}}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "test-id", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, testIDs, should.Match([]string{
				"test-id00",
				"test-id01",
				"test-id10",
				"test-id11",
				"test-id20",
				"test-id21",
			}))
		})

		t.Run("single-realm works", func(t *ftt.Test) {
			opts := QueryTestsOptions{SubRealms: []string{"realm3"}}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "test-id", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, testIDs, should.Match([]string{
				"test-id02",
				"test-id12",
				"test-id22",
			}))
		})

		t.Run("special character works", func(t *ftt.Test) {
			opts := QueryTestsOptions{SubRealms: []string{"realm1", "realm2", "realm3"}}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "special%_characters", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, testIDs, should.Match([]string{
				"special%_characters",
			}))
		})

		t.Run("case sensitive works", func(t *ftt.Test) {
			opts := QueryTestsOptions{SubRealms: []string{"realm1", "realm2", "realm3"}, CaseSensitive: true}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "CaSeSeNsItIvE", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, testIDs, should.BeEmpty)
		})
		t.Run("case insensitive works", func(t *ftt.Test) {
			opts := QueryTestsOptions{SubRealms: []string{"realm1", "realm2", "realm3"}, CaseSensitive: false}
			testIDs, nextPageToken, err := QueryTests(span.Single(ctx), "project", "CaSeSeNsItIvE", opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, testIDs, should.Match([]string{
				"caseSensitive",
			}))
		})
	})
}
