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
	"sort"
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
				StatusV2:          pb.TestVerdict_PASSED,
				StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
				PartitionTime:     timestamppb.New(referenceTime.Add(-1 * time.Hour)),
				PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant4),
				InvocationId:      "inv3",
				Status:            pb.TestVerdictStatus_EXPECTED,
				StatusV2:          pb.TestVerdict_SKIPPED,
				StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
				PartitionTime:     timestamppb.New(referenceTime.Add(-1 * time.Hour)),
				PassedAvgDuration: nil,
				Changelists:       expectedChangelists,
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant1),
				InvocationId:      "inv2",
				Status:            pb.TestVerdictStatus_EXONERATED,
				StatusV2:          pb.TestVerdict_FAILED,
				StatusOverride:    pb.TestVerdict_EXONERATED,
				PartitionTime:     timestamppb.New(referenceTime.Add(-12 * time.Hour)),
				PassedAvgDuration: nil,
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant2),
				InvocationId:      "inv1",
				Status:            pb.TestVerdictStatus_FLAKY,
				StatusV2:          pb.TestVerdict_FLAKY,
				StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
				PartitionTime:     timestamppb.New(referenceTime.Add(-24 * time.Hour)),
				PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant1),
				InvocationId:      "inv1",
				Status:            pb.TestVerdictStatus_UNEXPECTED,
				StatusV2:          pb.TestVerdict_FAILED,
				StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
				PartitionTime:     timestamppb.New(referenceTime.Add(-day - 1*time.Hour)),
				PassedAvgDuration: nil,
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant1),
				InvocationId:      "inv2",
				Status:            pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED,
				StatusV2:          pb.TestVerdict_EXECUTION_ERRORED,
				StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
				PartitionTime:     timestamppb.New(referenceTime.Add(-day - 12*time.Hour)),
				PassedAvgDuration: nil,
				Changelists:       expectedChangelists,
			}, {
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant2),
				InvocationId:      "inv1",
				Status:            pb.TestVerdictStatus_EXPECTED,
				StatusV2:          pb.TestVerdict_PASSED,
				StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
				PartitionTime:     timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
				PassedAvgDuration: nil,
				Changelists:       expectedChangelists,
			},
			{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant3),
				InvocationId:      "inv1",
				Status:            pb.TestVerdictStatus_EXONERATED,
				StatusV2:          pb.TestVerdict_PRECLUDED,
				StatusOverride:    pb.TestVerdict_EXONERATED,
				PartitionTime:     timestamppb.New(referenceTime.Add(-2*day - 3*time.Hour)),
				PassedAvgDuration: nil,
				Changelists:       expectedChangelists,
			},
		}

		now := referenceTime.Add(20 * time.Minute)

		readHistory := func(ctx context.Context, opts ReadTestHistoryOptions) ([]*pb.TestVerdict, string, error) {
			readTxn, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return ReadTestHistory(readTxn, opts, now)
		}

		t.Run("baseline", func(t *ftt.Test) {
			verdicts, nextPageToken, err := readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts))
		})
		t.Run("with legacy test results data", func(t *ftt.Test) {
			// This test case can be deleted from August 2025. This should be
			// combined with an update to make StatusV2 NOT NULL.
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				stmt := spanner.NewStatement("UPDATE TestResults SET StatusV2 = NULL WHERE TRUE")
				_, err := span.Update(ctx, stmt)
				return err
			})
			assert.Loosely(t, err, should.BeNil)

			// In test result status v1, precluded statuses cannot be represented.
			// The unexpected skips get mapped back to execution errors instead.
			expectedTestVerdicts[7].StatusV2 = pb.TestVerdict_EXECUTION_ERRORED

			verdicts, nextPageToken, err := readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts))
		})
		t.Run("pagination works", func(t *ftt.Test) {
			opts.PageSize = 5
			verdicts, nextPageToken, err := readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.NotBeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts[:5]))

			opts.PageToken = nextPageToken
			verdicts, nextPageToken, err = readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts[5:]))
		})

		t.Run("partial-page pagination when approaching deadline", func(t *ftt.Test) {
			// Insert a verdict 1 microsecond before the first partition boundary
			// (-24h - 1us) and a sparse verdict after an empty multi-day gap (-10d)
			// to verify boundary precision and empty-page continuation.
			boundaryMinus1us := referenceTime.Add(-day - time.Microsecond)
			sparseOldTime := referenceTime.Add(-10 * day)
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				for _, tr := range NewTestVerdict().
					WithBaseTestResult(NewTestResult().
						WithProject("project").
						WithTestID("test_id").
						WithVariantHash(pbutil.VariantHash(testVariant1)).
						WithPartitionTime(boundaryMinus1us).
						WithIngestedInvocationID("inv-boundary").
						WithSubRealm("realm").
						WithStatus(pb.TestResultStatus_PASS).
						WithSources(Sources{}).
						Build()).
					WithStatus(pb.TestVerdict_PASSED).
					WithStatusOverride(pb.TestVerdict_NOT_OVERRIDDEN).
					WithPassedAvgDuration(nil).
					Build() {
					span.BufferWrite(ctx, tr.SaveUnverified())
				}
				for _, tr := range NewTestVerdict().
					WithBaseTestResult(NewTestResult().
						WithProject("project").
						WithTestID("test_id").
						WithVariantHash(pbutil.VariantHash(testVariant2)).
						WithPartitionTime(sparseOldTime).
						WithIngestedInvocationID("inv-sparse").
						WithSubRealm("realm").
						WithStatus(pb.TestResultStatus_PASS).
						WithSources(Sources{}).
						Build()).
					WithStatus(pb.TestVerdict_PASSED).
					WithStatusOverride(pb.TestVerdict_NOT_OVERRIDDEN).
					WithPassedAvgDuration(nil).
					Build() {
					span.BufferWrite(ctx, tr.SaveUnverified())
				}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			opts.PageSize = 100
			readWithShortDeadline := func(opts ReadTestHistoryOptions) ([]*pb.TestVerdict, string, error) {
				shortCtx, cancel := context.WithTimeout(ctx, contextDeadlineSafetyMargin-500*time.Millisecond)
				defer cancel()
				return readHistory(shortCtx, opts)
			}

			// First call scans only the initial 2-day partition
			// [referenceTime - 24h, now) and returns a partial page (< PageSize)
			// including expectedTestVerdicts[3] right on the -24h boundary.
			firstPage, pageToken, err := readWithShortDeadline(opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pageToken, should.NotBeEmpty)
			assert.Loosely(t, firstPage, should.Match(expectedTestVerdicts[:4]))

			// Continue paginating until all 90 days are scanned, including
			// intermediate empty pages across the multi-day gap.
			allVerdicts := append([]*pb.TestVerdict{}, firstPage...)
			sawEmptyIntermediatePage := false
			pages := 1
			for pageToken != "" {
				opts.PageToken = pageToken
				page, nextTok, err := readWithShortDeadline(opts)
				assert.Loosely(t, err, should.BeNil)
				if len(page) == 0 && nextTok != "" {
					sawEmptyIntermediatePage = true
				}
				allVerdicts = append(allVerdicts, page...)
				pageToken = nextTok
				pages++
				assert.Loosely(t, pages, should.BeLessThan(60))
			}
			assert.Loosely(t, sawEmptyIntermediatePage, should.BeTrue)

			wantAll := make([]*pb.TestVerdict, 0, len(expectedTestVerdicts)+2)
			wantAll = append(wantAll, expectedTestVerdicts[:4]...)
			wantAll = append(wantAll, &pb.TestVerdict{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant1),
				InvocationId:      "inv-boundary",
				Status:            pb.TestVerdictStatus_EXPECTED,
				StatusV2:          pb.TestVerdict_PASSED,
				StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
				PartitionTime:     timestamppb.New(boundaryMinus1us),
				PassedAvgDuration: nil,
			})
			wantAll = append(wantAll, expectedTestVerdicts[4:]...)
			wantAll = append(wantAll, &pb.TestVerdict{
				TestId:            "test_id",
				VariantHash:       pbutil.VariantHash(testVariant2),
				InvocationId:      "inv-sparse",
				Status:            pb.TestVerdictStatus_EXPECTED,
				StatusV2:          pb.TestVerdict_PASSED,
				StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
				PartitionTime:     timestamppb.New(sparseOldTime),
				PassedAvgDuration: nil,
			})
			assert.Loosely(t, allVerdicts, should.Match(wantAll))
		})

		t.Run("no next_page_token when final partition finishes near deadline", func(t *ftt.Test) {
			opts.PageSize = 100
			opts.TimeRange = &pb.TimeRange{
				Earliest: timestamppb.New(referenceTime.Add(-day)),
				Latest:   timestamppb.New(now),
			}
			shortCtx, cancel := context.WithTimeout(ctx, contextDeadlineSafetyMargin-500*time.Millisecond)
			defer cancel()

			verdicts, nextPageToken, err := readHistory(shortCtx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts[:4]))
		})

		t.Run("with partition_time_range", func(t *ftt.Test) {
			opts.TimeRange = &pb.TimeRange{
				// Inclusive.
				Earliest: timestamppb.New(referenceTime.Add(-day - 24*time.Hour)),
				// Exclusive.
				Latest: timestamppb.New(referenceTime.Add(-24 * time.Hour)),
			}
			verdicts, nextPageToken, err := readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
				expectedTestVerdicts[4],
				expectedTestVerdicts[5],
				expectedTestVerdicts[6],
			}))
		})

		t.Run("with contains variant_predicate", func(t *ftt.Test) {
			t.Run("with single key-value pair", func(t *ftt.Test) {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2"),
					},
				}
				verdicts, nextPageToken, err := readHistory(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
					expectedTestVerdicts[3],
					expectedTestVerdicts[6],
					expectedTestVerdicts[7],
				}))
			})

			t.Run("with multiple key-value pairs", func(t *ftt.Test) {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2", "key2", "val2"),
					},
				}
				verdicts, nextPageToken, err := readHistory(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
					expectedTestVerdicts[7],
				}))
			})
		})

		t.Run("with equals variant_predicate", func(t *ftt.Test) {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{
					Equals: testVariant2,
				},
			}
			verdicts, nextPageToken, err := readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
				expectedTestVerdicts[3],
				expectedTestVerdicts[6],
			}))
		})

		t.Run("with hash_equals variant_predicate", func(t *ftt.Test) {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: pbutil.VariantHash(testVariant2),
				},
			}
			verdicts, nextPageToken, err := readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
				expectedTestVerdicts[3],
				expectedTestVerdicts[6],
			}))
		})

		t.Run("with submitted_filter", func(t *ftt.Test) {
			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_UNSUBMITTED
			verdicts, nextPageToken, err := readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
				expectedTestVerdicts[1],
				expectedTestVerdicts[5],
				expectedTestVerdicts[6],
				expectedTestVerdicts[7],
			}))

			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_SUBMITTED
			verdicts, nextPageToken, err = readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
				expectedTestVerdicts[0],
				expectedTestVerdicts[2],
				expectedTestVerdicts[3],
				expectedTestVerdicts[4],
			}))
		})

		t.Run("with bisection filter", func(t *ftt.Test) {
			opts.ExcludeBisectionResults = true
			verdicts, nextPageToken, err := readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.TestVerdict{
				expectedTestVerdicts[0],
				expectedTestVerdicts[2],
				expectedTestVerdicts[3],
				expectedTestVerdicts[4],
				expectedTestVerdicts[5],
				expectedTestVerdicts[6],
				expectedTestVerdicts[7],
			}))
		})
		t.Run("with previous_test_id", func(t *ftt.Test) {
			opts.PreviousTestID = "previous_test_id"
			verdicts, nextPageToken, err := readHistory(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)

			additionalExpectedTestVerdicts := []*pb.TestVerdict{
				{
					TestId:            "previous_test_id",
					VariantHash:       pbutil.VariantHash(testVariant5),
					InvocationId:      "inv-prev1",
					Status:            pb.TestVerdictStatus_UNEXPECTED,
					StatusV2:          pb.TestVerdict_FAILED,
					StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
				{
					TestId:            "previous_test_id",
					VariantHash:       pbutil.VariantHash(testVariant2),
					InvocationId:      "inv-prev1",
					Status:            pb.TestVerdictStatus_EXPECTED,
					StatusV2:          pb.TestVerdict_PASSED,
					StatusOverride:    pb.TestVerdict_NOT_OVERRIDDEN,
					PartitionTime:     timestamppb.New(referenceTime.Add(-day - 18*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
				{
					TestId:            "previous_test_id",
					VariantHash:       pbutil.VariantHash(testVariant3),
					InvocationId:      "ind-prev1",
					Status:            pb.TestVerdictStatus_EXONERATED,
					StatusV2:          pb.TestVerdict_PRECLUDED,
					StatusOverride:    pb.TestVerdict_EXONERATED,
					PartitionTime:     timestamppb.New(referenceTime.Add(-2*day - 6*time.Hour)),
					PassedAvgDuration: nil,
					Changelists:       expectedChangelists,
				},
			}
			expectedTestVerdicts = append(expectedTestVerdicts, additionalExpectedTestVerdicts...)
			// Sort test verdicts.
			sort.Slice(expectedTestVerdicts, func(i, j int) bool {
				a, b := expectedTestVerdicts[i], expectedTestVerdicts[j]
				if a.PartitionTime.AsTime() != (b.PartitionTime.AsTime()) {
					// Sort in desending time order.
					return a.PartitionTime.AsTime().After(b.PartitionTime.AsTime())
				}
				if a.TestId != b.TestId {
					return a.TestId < b.TestId
				}
				if a.VariantHash != b.VariantHash {
					return a.VariantHash < b.VariantHash
				}
				return a.InvocationId < b.InvocationId
			})

			assert.Loosely(t, verdicts, should.Match(expectedTestVerdicts))
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
				PassedAvgDuration: durationpb.New((22222) * time.Microsecond),
				VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
					Failed:           1, // which is also exonerated (see below).
					Passed:           1,
					FailedExonerated: 1,
				},
			},
			{
				PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
				VariantHash:   pbutil.VariantHash(testVariant4),
				ExpectedCount: 1,
				VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
					Skipped: 1,
				},
			},
			{
				PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
				VariantHash:       pbutil.VariantHash(testVariant2),
				FlakyCount:        1,
				PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
				VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
					Flaky: 1,
				},
			},
			{
				PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
				VariantHash:              pbutil.VariantHash(testVariant1),
				UnexpectedCount:          1,
				UnexpectedlySkippedCount: 1,
				VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
					Failed:           1,
					ExecutionErrored: 1,
				},
			},
			{
				PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
				VariantHash:       pbutil.VariantHash(testVariant2),
				ExpectedCount:     1,
				PassedAvgDuration: nil,
				VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
					Passed: 1,
				},
			},
			{
				PartitionTime:   timestamppb.New(referenceTime.Add(-3 * day)),
				VariantHash:     pbutil.VariantHash(testVariant3),
				ExoneratedCount: 1,
				VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
					Precluded:           1,
					PrecludedExonerated: 1,
				},
			},
		}
		now := referenceTime.Add(20 * time.Minute)

		readStats := func(ctx context.Context, opts ReadTestHistoryOptions) ([]*pb.QueryTestHistoryStatsResponse_Group, string, error) {
			readTxn, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return ReadTestHistoryStats(readTxn, opts, now)
		}

		t.Run("baseline", func(t *ftt.Test) {
			verdicts, nextPageToken, err := readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedGroups))
		})
		t.Run("pagination works", func(t *ftt.Test) {
			opts.PageSize = 4
			verdicts, nextPageToken, err := readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.NotBeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedGroups[:4]))

			opts.PageToken = nextPageToken
			verdicts, nextPageToken, err = readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedGroups[4:]))
		})

		t.Run("partial-page pagination when approaching deadline", func(t *ftt.Test) {
			// Add a sparse verdict at -10 days so there is a multi-day empty gap
			// between -3 days and -10 days.
			sparseOldTime := referenceTime.Add(-10*day + 2*time.Hour)
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				for _, tr := range NewTestVerdict().
					WithBaseTestResult(NewTestResult().
						WithProject("project").
						WithTestID("test_id").
						WithVariantHash(pbutil.VariantHash(testVariant2)).
						WithPartitionTime(sparseOldTime).
						WithIngestedInvocationID("inv-sparse").
						WithSubRealm("realm").
						WithStatus(pb.TestResultStatus_PASS).
						WithSources(Sources{}).
						Build()).
					WithStatus(pb.TestVerdict_PASSED).
					WithStatusOverride(pb.TestVerdict_NOT_OVERRIDDEN).
					WithPassedAvgDuration(nil).
					Build() {
					span.BufferWrite(ctx, tr.SaveUnverified())
				}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			opts.PageSize = 100
			readWithShortDeadline := func(opts ReadTestHistoryOptions) ([]*pb.QueryTestHistoryStatsResponse_Group, string, error) {
				shortCtx, cancel := context.WithTimeout(ctx, contextDeadlineSafetyMargin-500*time.Millisecond)
				defer cancel()
				return readStats(shortCtx, opts)
			}

			// First call scans [referenceTime - 1d, now) and returns only the
			// groups from day -1, plus a non-empty continuation token.
			firstPage, pageToken, err := readWithShortDeadline(opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pageToken, should.NotBeEmpty)
			assert.Loosely(t, firstPage, should.Match(expectedGroups[:3]))

			// Continue paginating across all remaining days, ensuring no day is
			// re-scanned or skipped and empty intermediate pages advance cleanly.
			allGroups := append([]*pb.QueryTestHistoryStatsResponse_Group{}, firstPage...)
			sawEmptyIntermediatePage := false
			pages := 1
			for pageToken != "" {
				opts.PageToken = pageToken
				page, nextTok, err := readWithShortDeadline(opts)
				assert.Loosely(t, err, should.BeNil)
				if len(page) == 0 && nextTok != "" {
					sawEmptyIntermediatePage = true
				}
				allGroups = append(allGroups, page...)
				pageToken = nextTok
				pages++
				assert.Loosely(t, pages, should.BeLessThan(60))
			}
			assert.Loosely(t, sawEmptyIntermediatePage, should.BeTrue)

			wantAll := append([]*pb.QueryTestHistoryStatsResponse_Group{}, expectedGroups...)
			wantAll = append(wantAll, &pb.QueryTestHistoryStatsResponse_Group{
				PartitionTime:     timestamppb.New(referenceTime.Add(-10 * day)),
				VariantHash:       pbutil.VariantHash(testVariant2),
				ExpectedCount:     1,
				PassedAvgDuration: nil,
				VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
					Passed: 1,
				},
			})
			assert.Loosely(t, allGroups, should.Match(wantAll))
		})

		t.Run("no next_page_token when final partition finishes near deadline", func(t *ftt.Test) {
			opts.PageSize = 100
			opts.TimeRange = &pb.TimeRange{
				Earliest: timestamppb.New(referenceTime.Add(-day)),
				Latest:   timestamppb.New(now),
			}
			shortCtx, cancel := context.WithTimeout(ctx, contextDeadlineSafetyMargin-500*time.Millisecond)
			defer cancel()

			groups, nextPageToken, err := readStats(shortCtx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, groups, should.Match(expectedGroups[:3]))
		})
		t.Run("with legacy test results data", func(t *ftt.Test) {
			// This test case can be deleted from August 2025. This should be
			// combined with an update to make StatusV2 NOT NULL.
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				stmt := spanner.NewStatement("UPDATE TestResults SET StatusV2 = NULL WHERE TRUE")
				_, err := span.Update(ctx, stmt)
				return err
			})
			assert.Loosely(t, err, should.BeNil)

			// In test result status v1, precluded statuses cannot be represented.
			// The unexpected skips get mapped back to execution errors.
			expectedGroups[5].VerdictCounts = &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
				ExecutionErrored:           1,
				ExecutionErroredExonerated: 1,
			}

			verdicts, nextPageToken, err := readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(expectedGroups))
		})
		t.Run("with previous_test_id", func(t *ftt.Test) {
			newExpectedGroups := []*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:   timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:     pbutil.VariantHash(testVariant5),
					UnexpectedCount: 1,
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Failed: 1,
					},
				},
				expectedGroups[0],
				expectedGroups[1],
				expectedGroups[2],
				expectedGroups[3],
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					ExpectedCount:     2,
					PassedAvgDuration: nil,
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Passed: 2,
					},
				},
				{
					PartitionTime:   timestamppb.New(referenceTime.Add(-3 * day)),
					VariantHash:     pbutil.VariantHash(testVariant3),
					ExoneratedCount: 2,
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Precluded:           2,
						PrecludedExonerated: 2,
					},
				},
			}

			opts.PreviousTestID = "previous_test_id"
			verdicts, nextPageToken, err := readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match(newExpectedGroups))
		})
		t.Run("with partition_time_range", func(t *ftt.Test) {
			t.Run("day boundaries", func(t *ftt.Test) {
				opts.TimeRange = &pb.TimeRange{
					// Inclusive.
					Earliest: timestamppb.New(referenceTime.Add(-2 * day)),
					// Exclusive.
					Latest: timestamppb.New(referenceTime.Add(-1 * day)),
				}
				verdicts, nextPageToken, err := readStats(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
					{
						PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:              pbutil.VariantHash(testVariant1),
						UnexpectedCount:          1,
						UnexpectedlySkippedCount: 1,
						PassedAvgDuration:        nil,
						VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
							Failed:           1,
							ExecutionErrored: 1,
						},
					},
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:       pbutil.VariantHash(testVariant2),
						ExpectedCount:     1,
						PassedAvgDuration: nil,
						VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
							Passed: 1,
						},
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
				verdicts, nextPageToken, err := readStats(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
					{
						PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:              pbutil.VariantHash(testVariant1),
						UnexpectedlySkippedCount: 1,
						PassedAvgDuration:        nil,
						VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
							ExecutionErrored: 1,
						},
					},
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:       pbutil.VariantHash(testVariant2),
						ExpectedCount:     1,
						PassedAvgDuration: nil,
						VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
							Passed: 1,
						},
					},
					{
						PartitionTime:   timestamppb.New(referenceTime.Add(-3 * day)),
						VariantHash:     pbutil.VariantHash(testVariant3),
						ExoneratedCount: 1,
						VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
							Precluded:           1,
							PrecludedExonerated: 1,
						},
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
				verdicts, nextPageToken, err := readStats(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
						VariantHash:       pbutil.VariantHash(testVariant2),
						FlakyCount:        1,
						PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
						VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
							Flaky: 1,
						},
					},
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
						VariantHash:       pbutil.VariantHash(testVariant2),
						ExpectedCount:     1,
						PassedAvgDuration: nil,
						VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
							Passed: 1,
						},
					},
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
						VariantHash:       pbutil.VariantHash(testVariant3),
						ExoneratedCount:   1,
						PassedAvgDuration: nil,
						VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
							Precluded:           1,
							PrecludedExonerated: 1,
						},
					},
				}))
			})

			t.Run("with multiple key-value pairs", func(t *ftt.Test) {
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2", "key2", "val2"),
					},
				}
				verdicts, nextPageToken, err := readStats(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextPageToken, should.BeEmpty)
				assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
					{
						PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
						VariantHash:       pbutil.VariantHash(testVariant3),
						ExoneratedCount:   1,
						PassedAvgDuration: nil,
						VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
							Precluded:           1,
							PrecludedExonerated: 1,
						},
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
			verdicts, nextPageToken, err := readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					FlakyCount:        1,
					PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Flaky: 1,
					},
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					ExpectedCount:     1,
					PassedAvgDuration: nil,
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Passed: 1,
					},
				},
			}))
		})

		t.Run("with hash_equals variant_predicate", func(t *ftt.Test) {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: pbutil.VariantHash(testVariant2),
				},
			}
			verdicts, nextPageToken, err := readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					FlakyCount:        1,
					PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Flaky: 1,
					},
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					ExpectedCount:     1,
					PassedAvgDuration: nil,
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Passed: 1,
					},
				},
			}))
		})

		t.Run("with empty hash_equals variant_predicate", func(t *ftt.Test) {
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: "",
				},
			}
			verdicts, nextPageToken, err := readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.BeEmpty)
		})

		t.Run("with submitted_filter", func(t *ftt.Test) {
			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_UNSUBMITTED
			verdicts, nextPageToken, err := readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant4),
					ExpectedCount:     1,
					PassedAvgDuration: nil,
					VerdictCounts:     &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Skipped: 1},
				},
				{
					PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:              pbutil.VariantHash(testVariant1),
					UnexpectedlySkippedCount: 1,
					PassedAvgDuration:        nil,
					VerdictCounts:            &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{ExecutionErrored: 1},
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					ExpectedCount:     1,
					PassedAvgDuration: nil,
					VerdictCounts:     &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Passed: 1},
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
					VariantHash:       pbutil.VariantHash(testVariant3),
					ExoneratedCount:   1,
					PassedAvgDuration: nil,
					VerdictCounts:     &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Precluded: 1, PrecludedExonerated: 1},
				},
			}))

			opts.SubmittedFilter = pb.SubmittedFilter_ONLY_SUBMITTED
			verdicts, nextPageToken, err = readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant1),
					ExpectedCount:     1,
					ExoneratedCount:   1,
					PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Failed:           1,
						Passed:           1,
						FailedExonerated: 1,
					},
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					FlakyCount:        1,
					PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Flaky: 1,
					},
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant1),
					UnexpectedCount:   1,
					PassedAvgDuration: nil,
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Failed: 1,
					},
				},
			}))
		})

		t.Run("with bisection filter", func(t *ftt.Test) {
			opts.ExcludeBisectionResults = true
			verdicts, nextPageToken, err := readStats(ctx, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, verdicts, should.Match([]*pb.QueryTestHistoryStatsResponse_Group{
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant1),
					ExpectedCount:     1,
					ExoneratedCount:   1,
					PassedAvgDuration: durationpb.New(22222 * time.Microsecond),
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Failed:           1,
						Passed:           1,
						FailedExonerated: 1,
					},
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-1 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					FlakyCount:        1,
					PassedAvgDuration: durationpb.New(1234567890123456 * time.Microsecond),
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Flaky: 1,
					},
				},
				{
					PartitionTime:            timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:              pbutil.VariantHash(testVariant1),
					UnexpectedCount:          1,
					UnexpectedlySkippedCount: 1,
					PassedAvgDuration:        nil,
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Failed:           1,
						ExecutionErrored: 1,
					},
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-2 * day)),
					VariantHash:       pbutil.VariantHash(testVariant2),
					ExpectedCount:     1,
					PassedAvgDuration: nil,
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Passed: 1,
					},
				},
				{
					PartitionTime:     timestamppb.New(referenceTime.Add(-3 * day)),
					VariantHash:       pbutil.VariantHash(testVariant3),
					ExoneratedCount:   1,
					PassedAvgDuration: nil,
					VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
						Precluded:           1,
						PrecludedExonerated: 1,
					},
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
		var5 := pbutil.Variant("keyold", "valOld")

		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			insertTVR := func(subRealm string, testID string, variant *pb.Variant) {
				span.BufferWrite(ctx, (&TestVariantRealm{
					Project:     "project",
					TestID:      testID,
					SubRealm:    subRealm,
					Variant:     variant,
					VariantHash: pbutil.VariantHash(variant),
				}).SaveUnverified())
			}

			insertTVR("realm1", "test_id", var1)
			insertTVR("realm1", "test_id", var2)
			insertTVR("realm1", "previous_test_id", var2)

			insertTVR("realm2", "test_id", var2)
			insertTVR("realm2", "previous_test_id", var2)
			insertTVR("realm2", "test_id", var3)
			insertTVR("realm2", "previous_test_id", var5)

			insertTVR("realm3", "test_id", var4)

			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		opts := ReadVariantsOptions{Project: "project", TestID: "test_id", SubRealms: []string{"realm1", "realm2"}}
		t.Run("pagination works", func(t *ftt.Test) {
			opts.SubRealms = []string{"realm1", "realm2", "realm3"}
			opts.PageSize = 3
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), opts)
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
			variants, nextPageToken, err = ReadVariants(span.Single(ctx), opts)
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
			opts.SubRealms = []string{"realm1", "realm2"}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), opts)
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
			opts.SubRealms = []string{"realm2"}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), opts)
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
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2"),
					},
				}
				variants, nextPageToken, err := ReadVariants(span.Single(ctx), opts)
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
				opts.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key1", "val2", "key2", "val2"),
					},
				}
				variants, nextPageToken, err := ReadVariants(span.Single(ctx), opts)
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
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{
					Equals: var2,
				},
			}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), opts)
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
			opts.VariantPredicate = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_HashEquals{
					HashEquals: pbutil.VariantHash(var2),
				},
			}
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)
			assert.Loosely(t, variants, should.Match([]*pb.QueryVariantsResponse_VariantInfo{
				{
					VariantHash: pbutil.VariantHash(var2),
					Variant:     var2,
				},
			}))
		})
		t.Run("with previous_test_id", func(t *ftt.Test) {
			opts.PreviousTestID = "previous_test_id"
			variants, nextPageToken, err := ReadVariants(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextPageToken, should.BeEmpty)

			expectedVariants := []*pb.QueryVariantsResponse_VariantInfo{
				// Sorted by variant hash.
				{VariantHash: pbutil.VariantHash(var1), Variant: var1},
				{VariantHash: pbutil.VariantHash(var3), Variant: var3},
				{VariantHash: pbutil.VariantHash(var5), Variant: var5},
				{VariantHash: pbutil.VariantHash(var2), Variant: var2},
			}
			assert.Loosely(t, variants, should.Match(expectedVariants))
		})
	})
}

func TestNextPartitionDays(t *testing.T) {
	t.Parallel()

	ftt.Run("nextPartitionDays", t, func(t *ftt.Test) {
		t.Run("without soft deadline uses item density capped at 31", func(t *ftt.Test) {
			days, ok := nextPartitionDays(0, 100, 1.5, 3*time.Second, false, 0)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, days, should.Equal(31))

			// 20 items over 2 days = 10 items/day; 40 remaining * 1.1 = 4.4 -> 5 days.
			days, ok = nextPartitionDays(20, 40, 2.0, 3*time.Second, false, 0)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, days, should.Equal(5))
		})

		t.Run("clamps daysRequired to fit remaining time budget", func(t *ftt.Test) {
			// 1.5 days took 3s (2s/day); 42s remaining -> at most 21 days fit (clamped from 31).
			days, ok := nextPartitionDays(0, 100, 1.5, 3*time.Second, true, 42*time.Second)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, days, should.Equal(21))

			// When item-density estimate (5 days) is smaller than budget (21 days), keep 5.
			days, ok = nextPartitionDays(20, 40, 2.0, 4*time.Second, true, 42*time.Second)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, days, should.Equal(5))
		})

		t.Run("allows exactly 1 day when 1 day fits", func(t *ftt.Test) {
			// 2 days took 10s (5s/day); 5s remaining -> exactly 1 day fits.
			days, ok := nextPartitionDays(0, 100, 2.0, 10*time.Second, true, 5*time.Second)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, days, should.Equal(1))
		})

		t.Run("stops early when less than 1 day fits", func(t *ftt.Test) {
			// 2 days took 10s (5s/day); 4s remaining -> < 1 day fits.
			days, ok := nextPartitionDays(0, 100, 2.0, 10*time.Second, true, 4*time.Second)
			assert.That(t, ok, should.BeFalse)
			assert.That(t, days, should.Equal(0))
		})

		t.Run("stops early when remaining time is non-positive", func(t *ftt.Test) {
			days, ok := nextPartitionDays(0, 100, 2.0, 0, true, 0)
			assert.That(t, ok, should.BeFalse)
			assert.That(t, days, should.Equal(0))

			days, ok = nextPartitionDays(0, 100, 2.0, 0, true, -500*time.Millisecond)
			assert.That(t, ok, should.BeFalse)
			assert.That(t, days, should.Equal(0))
		})

		t.Run("handles zero query duration with positive remaining time", func(t *ftt.Test) {
			days, ok := nextPartitionDays(0, 100, 2.0, 0, true, 10*time.Second)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, days, should.Equal(31))
		})
	})
}
