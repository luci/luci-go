// Copyright 2023 The LUCI Authors.
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

package testvariantbranch

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/model"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	tu "go.chromium.org/luci/analysis/internal/changepoints/testutil"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestFetchUpdateTestVariantBranch(t *testing.T) {
	ftt.Run("Fetch not found", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		key := Key{
			Project:     "proj",
			TestID:      "test_id",
			VariantHash: "variant_hash",
			RefHash:     "git_hash",
		}
		tvbs, err := Read(span.Single(ctx), []Key{key})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(tvbs), should.Equal(1))
		assert.Loosely(t, tvbs[0], should.BeNil)
	})

	ftt.Run("Insert and fetch", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		tvb1 := &Entry{
			IsNew:       true,
			Project:     "proj_1",
			TestID:      "test_id_1",
			VariantHash: "variant_hash_1",
			RefHash:     []byte("refhash1"),
			Variant: &pb.Variant{
				Def: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			SourceRef: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host_1",
						Project: "proj_1",
						Ref:     "ref_1",
					},
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity: 100,
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						{
							CommitPosition: 15,
							Hour:           time.Unix(0, 0),
							Expected: inputbuffer.ResultCounts{
								PassCount: 1,
							},
						},
						{
							CommitPosition: 18,
							Hour:           time.Unix(0, 0),
							Expected: inputbuffer.ResultCounts{
								PassCount:  1,
								FailCount:  2,
								CrashCount: 3,
								AbortCount: 4,
							},
							Unexpected: inputbuffer.ResultCounts{
								PassCount:  5,
								FailCount:  6,
								CrashCount: 7,
								AbortCount: 8,
							},
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
		}

		tvb3 := &Entry{
			IsNew:       true,
			Project:     "proj_3",
			TestID:      "test_id_3",
			VariantHash: "variant_hash_3",
			Variant:     &pb.Variant{},
			RefHash:     []byte("refhash3"),
			SourceRef: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host_3",
						Project: "proj_3",
						Ref:     "ref_3",
					},
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity: 100,
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						{
							CommitPosition: 20,
							Hour:           time.Unix(0, 0),
							Expected: inputbuffer.ResultCounts{
								PassCount: 1,
							},
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
		}

		var hs inputbuffer.HistorySerializer
		mutation1, err := tvb1.ToMutation(&hs)
		assert.Loosely(t, err, should.BeNil)
		mutation3, err := tvb3.ToMutation(&hs)
		assert.Loosely(t, err, should.BeNil)
		testutil.MustApply(ctx, t, mutation1, mutation3)

		tvbks := []Key{
			makeKey("proj_1", "test_id_1", "variant_hash_1", "refhash1"),
			makeKey("proj_2", "test_id_2", "variant_hash_2", "refhash2"),
			makeKey("proj_3", "test_id_3", "variant_hash_3", "refhash3"),
		}
		tvbs, err := Read(span.Single(ctx), tvbks)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(tvbs), should.Equal(3))
		// After inserting, the record should not be new anymore.
		tvb1.IsNew = false
		// After decoding, cold buffer should be empty.
		tvb1.InputBuffer.ColdBuffer = inputbuffer.History{Runs: []inputbuffer.Run{}}

		assert.Loosely(t, tvbs[0], should.Match(tvb1))

		assert.Loosely(t, tvbs[1], should.BeNil)

		tvb3.IsNew = false
		tvb3.InputBuffer.ColdBuffer = inputbuffer.History{Runs: []inputbuffer.Run{}}

		assert.Loosely(t, tvbs[2], should.Match(tvb3))
	})

	ftt.Run("Insert and update", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		// Insert a new record.
		tvb := &Entry{
			IsNew:       true,
			Project:     "proj_1",
			TestID:      "test_id_1",
			VariantHash: "variant_hash_1",
			RefHash:     []byte("githash1"),
			Variant: &pb.Variant{
				Def: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			SourceRef: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host_1",
						Project: "proj_1",
						Ref:     "ref_1",
					},
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity: 100,
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						{
							CommitPosition: 15,
							Hour:           time.Unix(0, 0),
							Expected: inputbuffer.ResultCounts{
								PassCount: 1,
							},
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
		}

		var hs inputbuffer.HistorySerializer
		mutation, err := tvb.ToMutation(&hs)
		assert.Loosely(t, err, should.BeNil)
		testutil.MustApply(ctx, t, mutation)

		// Update the record
		tvb = &Entry{
			Project:     "proj_1",
			TestID:      "test_id_1",
			VariantHash: "variant_hash_1",
			RefHash:     []byte("githash1"),
			Variant: &pb.Variant{
				Def: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			SourceRef: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host_1",
						Project: "proj_1",
						Ref:     "ref_1",
					},
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity: 100,
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						{
							CommitPosition: 16,
							Hour:           time.Unix(0, 0),
							Expected: inputbuffer.ResultCounts{
								PassCount: 1,
							},
						},
					},
				},
				ColdBufferCapacity: 2000,
				ColdBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						{
							CommitPosition: 15,
							Hour:           time.Unix(0, 0),
							Expected: inputbuffer.ResultCounts{
								PassCount:  1,
								FailCount:  2,
								CrashCount: 3,
								AbortCount: 4,
							},
							Unexpected: inputbuffer.ResultCounts{
								PassCount:  5,
								FailCount:  6,
								CrashCount: 7,
								AbortCount: 8,
							},
						},
					},
				},
				IsColdBufferDirty: true,
			},
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				HasStartChangepoint:          true,
				StartPosition:                50,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				StartPositionLowerBound_99Th: 45,
				StartPositionUpperBound_99Th: 55,
				StartPositionDistribution:    model.SimpleDistribution(50, 5).Serialize(),
				FinalizedCounts: &cpb.Counts{
					TotalResults: 10,
					TotalRuns:    10,
					FlakyRuns:    10,
				},
			},
			IsFinalizingSegmentDirty: true,
			FinalizedSegments: &cpb.Segments{
				Segments: []*cpb.Segment{
					{
						State:                        cpb.SegmentState_FINALIZED,
						HasStartChangepoint:          true,
						StartPosition:                20,
						StartHour:                    timestamppb.New(time.Unix(3600, 0)),
						StartPositionLowerBound_99Th: 10,
						StartPositionUpperBound_99Th: 30,
						StartPositionDistribution:    model.SimpleDistribution(20, 10).Serialize(),
						EndPosition:                  40,
						EndHour:                      timestamppb.New(time.Unix(3600, 0)),
						FinalizedCounts: &cpb.Counts{
							TotalResults:             10,
							TotalRuns:                10,
							FlakyRuns:                10,
							UnexpectedResults:        10,
							ExpectedPassedResults:    1,
							ExpectedFailedResults:    2,
							ExpectedCrashedResults:   3,
							ExpectedAbortedResults:   4,
							UnexpectedPassedResults:  5,
							UnexpectedFailedResults:  6,
							UnexpectedCrashedResults: 7,
							UnexpectedAbortedResults: 8,
							PartialSourceVerdict: &cpb.PartialSourceVerdict{
								CommitPosition:    987654321987654321,
								LastHour:          timestamppb.New(time.Date(2222, time.February, 2, 22, 0, 0, 0, time.UTC)),
								ExpectedResults:   66666,
								UnexpectedResults: 11111,
							},
						},
					},
				},
			},
			IsFinalizedSegmentsDirty: true,
			Statistics: &cpb.Statistics{
				HourlyBuckets: []*cpb.Statistics_HourBucket{{
					Hour:                     100,
					UnexpectedSourceVerdicts: 1,
					FlakySourceVerdicts:      2,
					TotalSourceVerdicts:      4,
				}},
				PartialSourceVerdict: &cpb.PartialSourceVerdict{
					CommitPosition:    1234567890123456789,
					LastHour:          timestamppb.New(time.Date(2123, time.January, 1, 15, 0, 0, 0, time.UTC)),
					ExpectedResults:   777,
					UnexpectedResults: 888,
				},
			},
			IsStatisticsDirty: true,
		}

		mutation, err = tvb.ToMutation(&hs)
		assert.Loosely(t, err, should.BeNil)
		testutil.MustApply(ctx, t, mutation)

		tvbks := []Key{
			makeKey("proj_1", "test_id_1", "variant_hash_1", "githash1"),
		}
		tvbs, err := Read(span.Single(ctx), tvbks)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(tvbs), should.Equal(1))

		tvb.IsStatisticsDirty = false
		tvb.IsFinalizedSegmentsDirty = false
		tvb.IsFinalizingSegmentDirty = false
		tvb.InputBuffer.IsColdBufferDirty = false

		assert.Loosely(t, tvbs[0], should.Match(tvb))
	})
}

func TestInsertToInputBuffer(t *testing.T) {
	ftt.Run("Insert simple test variant", t, func(t *ftt.Test) {
		tvb := &Entry{
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  10,
				ColdBufferCapacity: 100,
			},
		}
		partitionTime := time.Date(2031, time.January, 1, 15, 4, 0, 12, time.UTC)
		sourcesMap := tu.SampleSourcesMap(12)
		tv := &rdbpb.TestVariant{
			Status: rdbpb.TestVariantStatus_EXPECTED,
			Results: []*rdbpb.TestResultBundle{
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-1/tests/abc",
						Expected: true,
						Status:   rdbpb.TestStatus_PASS,
					},
				},
			},
			SourcesId: "sources_id",
		}
		runs, err := ToRuns(tv, partitionTime, map[string]bool{"run-1": true}, sourcesMap["sources_id"])
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, runs, should.HaveLength(1))
		tvb.InsertToInputBuffer(runs[0])
		assert.Loosely(t, len(tvb.InputBuffer.HotBuffer.Runs), should.Equal(1))

		assert.Loosely(t, tvb.InputBuffer.HotBuffer.Runs[0], should.Match(inputbuffer.Run{
			CommitPosition: 12,
			Hour:           time.Date(2031, time.January, 1, 15, 0, 0, 0, time.UTC),
			Expected: inputbuffer.ResultCounts{
				PassCount: 1,
			},
		}))
	})

	ftt.Run("Insert non-simple test variant", t, func(t *ftt.Test) {
		tvb := &Entry{
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  10,
				ColdBufferCapacity: 100,
			},
		}

		sourcesMap := tu.SampleSourcesMap(12)
		tv := &rdbpb.TestVariant{
			Status:    rdbpb.TestVariantStatus_FLAKY,
			SourcesId: "sources_id",
			Results: []*rdbpb.TestResultBundle{
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-1/tests/abc",
						Expected: true,
						Status:   rdbpb.TestStatus_PASS,
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-1/tests/abc",
						Expected: true,
						Status:   rdbpb.TestStatus_FAIL,
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-1/tests/abc",
						Expected: true,
						Status:   rdbpb.TestStatus_CRASH,
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-2/tests/abc",
						Expected: true,
						Status:   rdbpb.TestStatus_ABORT,
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-3/tests/abc",
						Expected: false,
						Status:   rdbpb.TestStatus_PASS,
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-3/tests/abc",
						Expected: false,
						Status:   rdbpb.TestStatus_FAIL,
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-4/tests/abc",
						Expected: false,
						Status:   rdbpb.TestStatus_CRASH,
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-4/tests/abc",
						Expected: false,
						Status:   rdbpb.TestStatus_ABORT,
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:     "invocations/run-5/tests/abc",
						Expected: true,
						Status:   rdbpb.TestStatus_PASS,
					},
				},
			},
		}
		claimedInvs := map[string]bool{
			"run-1": true,
			"run-2": true,
			"run-3": true,
			"run-4": true,
		}
		partitionTime := time.Date(2031, time.January, 1, 15, 4, 0, 12, time.UTC)

		runs, err := ToRuns(tv, partitionTime, claimedInvs, sourcesMap["sources_id"])
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, runs, should.HaveLength(4))
		for _, run := range runs {
			tvb.InsertToInputBuffer(run)
		}
		assert.Loosely(t, len(tvb.InputBuffer.HotBuffer.Runs), should.Equal(4))

		// Insertion reverses the order of runs as they are preferentially
		// added to the end of of the buffer.
		assert.Loosely(t, tvb.InputBuffer.HotBuffer.Runs, should.Match([]inputbuffer.Run{
			{
				// run-4
				CommitPosition: 12,
				Hour:           time.Date(2031, time.January, 1, 15, 0, 0, 0, time.UTC),
				Unexpected: inputbuffer.ResultCounts{
					CrashCount: 1,
					AbortCount: 1,
				},
			},
			{
				// run-3
				CommitPosition: 12,
				Hour:           time.Date(2031, time.January, 1, 15, 0, 0, 0, time.UTC),
				Unexpected: inputbuffer.ResultCounts{
					PassCount: 1,
					FailCount: 1,
				},
			},
			{
				// run-2
				CommitPosition: 12,
				Hour:           time.Date(2031, time.January, 1, 15, 0, 0, 0, time.UTC),
				Expected: inputbuffer.ResultCounts{
					AbortCount: 1,
				},
			},
			{
				// run-1
				CommitPosition: 12,
				Hour:           time.Date(2031, time.January, 1, 15, 0, 0, 0, time.UTC),
				Expected: inputbuffer.ResultCounts{
					PassCount:  1,
					FailCount:  1,
					CrashCount: 1,
				},
			},
		}))
	})
}

func TestUpdateOutputBuffer(t *testing.T) {
	ftt.Run("No existing finalizing segment", t, func(t *ftt.Test) {
		tvb := Entry{}
		evictedSegments := []inputbuffer.EvictedSegment{
			{
				State:         cpb.SegmentState_FINALIZED,
				StartPosition: 1,
				StartHour:     time.Unix(1*3600, 0),
				EndPosition:   10,
				EndHour:       time.Unix(10*3600, 0),
				Runs: []*inputbuffer.Run{
					{
						CommitPosition: 2,
						Hour:           time.Unix(10*3600, 0),
						Expected: inputbuffer.ResultCounts{
							PassCount:  1,
							FailCount:  2,
							CrashCount: 3,
							AbortCount: 4,
						},
						Unexpected: inputbuffer.ResultCounts{
							PassCount:  5,
							FailCount:  6,
							CrashCount: 7,
							AbortCount: 8,
						},
					},
				},
				MostRecentUnexpectedResultHour: time.Unix(7*3600, 0),
			},
			{
				State:                          cpb.SegmentState_FINALIZING,
				HasStartChangepoint:            true,
				StartPosition:                  11,
				StartHour:                      time.Unix(11*3600, 0),
				StartPositionDistribution:      model.SimpleDistribution(11, 2),
				MostRecentUnexpectedResultHour: time.Unix((100000-StatisticsRetentionDays*24)*3600, 0),
				Runs: []*inputbuffer.Run{
					{
						CommitPosition: 11,
						Hour:           time.Unix(11*3600, 0),
						Expected: inputbuffer.ResultCounts{
							PassCount: 1,
						},
					},
					{
						CommitPosition: 11,
						Hour:           time.Unix(100000*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							CrashCount: 1,
						},
					},
					{
						CommitPosition: 12,
						Hour:           time.Unix((100000-StatisticsRetentionDays*24)*3600, 0),
						Expected: inputbuffer.ResultCounts{
							PassCount: 1,
						},
					},
					{
						CommitPosition: 13,
						Hour:           time.Unix((100000-StatisticsRetentionDays*24+1)*3600, 0),
						Expected: inputbuffer.ResultCounts{
							PassCount: 1,
						},
						Unexpected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
				},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)

		assert.Loosely(t, tvb.FinalizingSegment, should.NotBeNil)
		assert.Loosely(t, tvb.FinalizingSegment, should.Match(&cpb.Segment{
			State:                        cpb.SegmentState_FINALIZING,
			HasStartChangepoint:          true,
			StartPosition:                11,
			StartHour:                    timestamppb.New(time.Unix(11*3600, 0)),
			StartPositionLowerBound_99Th: 9,
			StartPositionUpperBound_99Th: 13,
			StartPositionDistribution:    model.SimpleDistribution(11, 2).Serialize(),
			FinalizedCounts: &cpb.Counts{
				UnexpectedResults:        2,
				TotalResults:             5,
				ExpectedPassedResults:    3,
				ExpectedFailedResults:    0,
				ExpectedCrashedResults:   0,
				ExpectedAbortedResults:   0,
				UnexpectedPassedResults:  0,
				UnexpectedFailedResults:  1,
				UnexpectedCrashedResults: 1,
				UnexpectedAbortedResults: 0,

				TotalRuns:                4,
				UnexpectedUnretriedRuns:  1,
				UnexpectedAfterRetryRuns: 0,
				FlakyRuns:                1,

				TotalSourceVerdicts:      2,
				FlakySourceVerdicts:      1,
				UnexpectedSourceVerdicts: 0,
				PartialSourceVerdict: &cpb.PartialSourceVerdict{
					CommitPosition:    13,
					LastHour:          timestamppb.New(time.Unix((100000-StatisticsRetentionDays*24+1)*3600, 0)),
					UnexpectedResults: 1,
					ExpectedResults:   1,
				},
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix((100000-StatisticsRetentionDays*24)*3600, 0)),
		}))
		assert.Loosely(t, tvb.IsFinalizingSegmentDirty, should.BeTrue)

		assert.Loosely(t, len(tvb.FinalizedSegments.Segments), should.Equal(1))
		assert.Loosely(t, tvb.FinalizedSegments.Segments[0], should.Match(&cpb.Segment{
			State:               cpb.SegmentState_FINALIZED,
			StartPosition:       1,
			StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
			EndPosition:         10,
			EndHour:             timestamppb.New(time.Unix(10*3600, 0)),
			HasStartChangepoint: false,
			FinalizedCounts: &cpb.Counts{
				TotalResults:             36,
				UnexpectedResults:        26,
				ExpectedPassedResults:    1,
				ExpectedFailedResults:    2,
				ExpectedCrashedResults:   3,
				ExpectedAbortedResults:   4,
				UnexpectedPassedResults:  5,
				UnexpectedFailedResults:  6,
				UnexpectedCrashedResults: 7,
				UnexpectedAbortedResults: 8,

				TotalRuns: 1,
				FlakyRuns: 1,

				TotalSourceVerdicts: 1,
				FlakySourceVerdicts: 1,
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(7*3600, 0)),
		})) // evictedSegments[0].Segment
		assert.Loosely(t, tvb.IsFinalizedSegmentsDirty, should.BeTrue)

		assert.Loosely(t, tvb.Statistics, should.Match(&cpb.Statistics{
			HourlyBuckets: []*cpb.Statistics_HourBucket{
				// Confirm that buckets for hours 11 and (100000 - StatisticsRetentionDays*24)
				// are not present due to retention policies.
				{
					Hour:                100000,
					TotalSourceVerdicts: 1,
					FlakySourceVerdicts: 1, // commit position 11.
				},
			},
			PartialSourceVerdict: &cpb.PartialSourceVerdict{
				CommitPosition:    13,
				LastHour:          timestamppb.New(time.Unix((100000-StatisticsRetentionDays*24+1)*3600, 0)),
				ExpectedResults:   1,
				UnexpectedResults: 1,
			},
		}))
		assert.Loosely(t, tvb.IsStatisticsDirty, should.BeTrue)
	})

	ftt.Run("Combine finalizing segment with finalizing segment", t, func(t *ftt.Test) {
		tvb := Entry{
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
				StartPositionDistribution:    model.SimpleDistribution(100, 10).Serialize(),
				FinalizedCounts: &cpb.Counts{
					TotalResults:             30,
					UnexpectedResults:        5,
					ExpectedPassedResults:    1,
					ExpectedFailedResults:    2,
					ExpectedCrashedResults:   3,
					ExpectedAbortedResults:   4,
					UnexpectedPassedResults:  5,
					UnexpectedFailedResults:  6,
					UnexpectedCrashedResults: 7,
					UnexpectedAbortedResults: 8,

					TotalRuns:                20,
					UnexpectedUnretriedRuns:  2,
					UnexpectedAfterRetryRuns: 3,
					FlakyRuns:                4,

					TotalSourceVerdicts:      10,
					UnexpectedSourceVerdicts: 1,
					FlakySourceVerdicts:      2,
					PartialSourceVerdict: &cpb.PartialSourceVerdict{
						CommitPosition:    200,
						LastHour:          timestamppb.New(time.Unix(3*3600, 0)),
						UnexpectedResults: 1,
						ExpectedResults:   2,
					},
				},
				MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(7*3600, 0)),
			},
		}
		evictedSegments := []inputbuffer.EvictedSegment{
			{
				State:                          cpb.SegmentState_FINALIZING,
				StartPosition:                  200,
				StartHour:                      time.Unix(100*3600, 0),
				HasStartChangepoint:            false,
				MostRecentUnexpectedResultHour: time.Unix(10*3600, 0),
				Runs: []*inputbuffer.Run{
					{
						CommitPosition: 200,
						Hour:           time.Unix(5*3600, 0),
						Expected: inputbuffer.ResultCounts{
							PassCount:  100,
							FailCount:  200,
							CrashCount: 300,
							AbortCount: 400,
						},
						Unexpected: inputbuffer.ResultCounts{
							PassCount:  500,
							FailCount:  600,
							CrashCount: 700,
							AbortCount: 800,
						},
					},
					{
						CommitPosition: 250,
						Hour:           time.Unix(15*3600, 0),
						Expected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
				},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)
		assert.Loosely(t, tvb.FinalizedSegments, should.BeNil)
		assert.Loosely(t, tvb.FinalizingSegment, should.NotBeNil)

		expected := &cpb.Segment{
			State:                        cpb.SegmentState_FINALIZING,
			StartPosition:                100,
			StartHour:                    timestamppb.New(time.Unix(3600, 0)),
			HasStartChangepoint:          true,
			StartPositionLowerBound_99Th: 90,
			StartPositionUpperBound_99Th: 110,
			StartPositionDistribution:    model.SimpleDistribution(100, 10).Serialize(),
			FinalizedCounts: &cpb.Counts{
				TotalResults:             3631, // 30+(100+200+300+400+500+600+700+800)+1
				UnexpectedResults:        2605, // 5+500+600+700+800
				ExpectedPassedResults:    101,
				ExpectedFailedResults:    203, // 2+200+1
				ExpectedCrashedResults:   303,
				ExpectedAbortedResults:   404,
				UnexpectedPassedResults:  505,
				UnexpectedFailedResults:  606,
				UnexpectedCrashedResults: 707,
				UnexpectedAbortedResults: 808,

				TotalRuns:                22, // 20+2
				UnexpectedUnretriedRuns:  2,  // unchanged
				UnexpectedAfterRetryRuns: 3,  // unchanged
				FlakyRuns:                5,  // 4+1

				TotalSourceVerdicts:      11, // one more, for commit position 200.
				UnexpectedSourceVerdicts: 1,
				FlakySourceVerdicts:      3, // one more, for commit position 200.

				PartialSourceVerdict: &cpb.PartialSourceVerdict{
					CommitPosition:  250,
					LastHour:        timestamppb.New(time.Unix(15*3600, 0)),
					ExpectedResults: 1,
				},
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
		}
		assert.Loosely(t, tvb.FinalizingSegment, should.Match(expected))
	})

	ftt.Run("Combine finalizing segment with finalized segment", t, func(t *ftt.Test) {
		tvb := Entry{
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
				StartPositionDistribution:    model.SimpleDistribution(100, 10).Serialize(),
				FinalizedCounts: &cpb.Counts{
					TotalResults:             30,
					UnexpectedResults:        5,
					TotalRuns:                20,
					ExpectedPassedResults:    1,
					ExpectedFailedResults:    2,
					ExpectedCrashedResults:   3,
					ExpectedAbortedResults:   4,
					UnexpectedPassedResults:  5,
					UnexpectedFailedResults:  6,
					UnexpectedCrashedResults: 7,
					UnexpectedAbortedResults: 8,

					UnexpectedUnretriedRuns:  2,
					UnexpectedAfterRetryRuns: 3,
					FlakyRuns:                4,

					TotalSourceVerdicts:      10,
					UnexpectedSourceVerdicts: 1,
					FlakySourceVerdicts:      2,
					PartialSourceVerdict: &cpb.PartialSourceVerdict{
						CommitPosition:    200,
						LastHour:          timestamppb.New(time.Unix(3*3600, 0)),
						UnexpectedResults: 1,
						ExpectedResults:   2,
					},
				},
				MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(7*3600, 0)),
			},
		}
		evictedSegments := []inputbuffer.EvictedSegment{
			{
				State:                          cpb.SegmentState_FINALIZED,
				StartPosition:                  200,
				StartHour:                      time.Unix(100*3600, 0),
				HasStartChangepoint:            false,
				EndPosition:                    400,
				EndHour:                        time.Unix(400*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(10*3600, 0),
				Runs: []*inputbuffer.Run{
					{
						CommitPosition: 200,
						Hour:           time.Unix(5*3600, 0),
						Expected: inputbuffer.ResultCounts{
							PassCount:  100,
							FailCount:  200,
							CrashCount: 300,
							AbortCount: 400,
						},
						Unexpected: inputbuffer.ResultCounts{
							PassCount:  500,
							FailCount:  600,
							CrashCount: 700,
							AbortCount: 800,
						},
					},
					{
						CommitPosition: 250,
						Hour:           time.Unix(15*3600, 0),
						Expected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
				},
			},
			{
				State:                     cpb.SegmentState_FINALIZING,
				HasStartChangepoint:       true,
				StartPosition:             500,
				StartHour:                 time.Unix(20*3600, 0),
				StartPositionDistribution: model.SimpleDistribution(500, 10),
				EndPosition:               800,
				Runs: []*inputbuffer.Run{
					{
						CommitPosition: 500,
						Hour:           time.Unix(20*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							CrashCount: 2,
						},
					},
					{
						CommitPosition: 510,
						Hour:           time.Unix(25*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							CrashCount: 4,
						},
					},
				},
				MostRecentUnexpectedResultHour: time.Unix(25*3600, 0),
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)

		expectedFinalizedSegment := &cpb.Segment{
			State:                        cpb.SegmentState_FINALIZED,
			StartPosition:                100,
			StartHour:                    timestamppb.New(time.Unix(3600, 0)),
			HasStartChangepoint:          true,
			StartPositionLowerBound_99Th: 90,
			StartPositionUpperBound_99Th: 110,
			StartPositionDistribution:    model.SimpleDistribution(100, 10).Serialize(),
			EndPosition:                  400,
			EndHour:                      timestamppb.New(time.Unix(400*3600, 0)),
			FinalizedCounts: &cpb.Counts{
				TotalResults:             3631, // 30+(100+200+300+400+500+600+700+800)+1
				UnexpectedResults:        2605, // 5+500+600+700+800,
				ExpectedPassedResults:    101,
				ExpectedFailedResults:    203, // 2 (original segment) + 200+1 (runs from evicted segment).
				ExpectedCrashedResults:   303,
				ExpectedAbortedResults:   404,
				UnexpectedPassedResults:  505,
				UnexpectedFailedResults:  606,
				UnexpectedCrashedResults: 707,
				UnexpectedAbortedResults: 808,

				TotalRuns:                22, // 20+2
				UnexpectedUnretriedRuns:  2,  // unchanged
				UnexpectedAfterRetryRuns: 3,  // unchanged
				FlakyRuns:                5,  // 4+1

				TotalSourceVerdicts:      12, // two more, for commit position 200 and 250.
				UnexpectedSourceVerdicts: 1,
				FlakySourceVerdicts:      3, // one more, for commit position 200.
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
		}
		assert.Loosely(t, len(tvb.FinalizedSegments.Segments), should.Equal(1))
		assert.Loosely(t, tvb.FinalizedSegments.Segments[0], should.Match(expectedFinalizedSegment))

		expectedFinalizingSegment := &cpb.Segment{
			State:                        cpb.SegmentState_FINALIZING,
			HasStartChangepoint:          true,
			StartPosition:                500,
			StartHour:                    timestamppb.New(time.Unix(20*3600, 0)),
			StartPositionLowerBound_99Th: 490,
			StartPositionUpperBound_99Th: 510,
			StartPositionDistribution:    model.SimpleDistribution(500, 10).Serialize(),
			FinalizedCounts: &cpb.Counts{
				TotalResults:             6,
				UnexpectedResults:        6,
				UnexpectedCrashedResults: 6,

				TotalRuns:                2,
				UnexpectedAfterRetryRuns: 2,

				TotalSourceVerdicts:      1,
				UnexpectedSourceVerdicts: 1,
				PartialSourceVerdict: &cpb.PartialSourceVerdict{
					CommitPosition:    510,
					LastHour:          timestamppb.New(time.Unix(25*3600, 0)),
					UnexpectedResults: 4,
				},
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(25*3600, 0)),
		}
		assert.Loosely(t, tvb.FinalizingSegment, should.Match(expectedFinalizingSegment))
	})

	ftt.Run("Combine finalizing segment with finalized segment, with a token finalizing segment in input buffer", t, func(t *ftt.Test) {
		tvb := Entry{
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
				StartPositionDistribution:    model.SimpleDistribution(100, 10).Serialize(),
				FinalizedCounts: &cpb.Counts{
					TotalResults:             30,
					UnexpectedResults:        5,
					ExpectedPassedResults:    1,
					ExpectedFailedResults:    2,
					ExpectedCrashedResults:   3,
					ExpectedAbortedResults:   4,
					UnexpectedPassedResults:  5,
					UnexpectedFailedResults:  6,
					UnexpectedCrashedResults: 7,
					UnexpectedAbortedResults: 8,

					TotalRuns:                20,
					UnexpectedUnretriedRuns:  2,
					UnexpectedAfterRetryRuns: 3,
					FlakyRuns:                4,

					TotalSourceVerdicts:      10,
					UnexpectedSourceVerdicts: 1,
					FlakySourceVerdicts:      2,
					PartialSourceVerdict: &cpb.PartialSourceVerdict{
						CommitPosition:    200,
						LastHour:          timestamppb.New(time.Unix(3*3600, 0)),
						UnexpectedResults: 0,
						ExpectedResults:   2,
					},
				},
				MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(7*3600, 0)),
			},
		}
		evictedSegments := []inputbuffer.EvictedSegment{
			{
				State:                          cpb.SegmentState_FINALIZED,
				StartPosition:                  200,
				StartHour:                      time.Unix(100*3600, 0),
				HasStartChangepoint:            false,
				EndPosition:                    400,
				EndHour:                        time.Unix(400*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(10*3600, 0),
				Runs: []*inputbuffer.Run{
					{
						CommitPosition: 200,
						Hour:           time.Unix(100*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							CrashCount: 1,
						},
					},
					{
						CommitPosition: 400,
						Hour:           time.Unix(90*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							CrashCount: 1,
						},
					},
				},
			},
			{
				State:                     cpb.SegmentState_FINALIZING,
				StartPosition:             500,
				StartHour:                 time.Unix(500*3600, 0),
				HasStartChangepoint:       true,
				StartPositionDistribution: model.SimpleDistribution(500, 10),
				Runs:                      []*inputbuffer.Run{},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)

		expectedFinalized := &cpb.Segment{
			State:                        cpb.SegmentState_FINALIZED,
			StartPosition:                100,
			StartHour:                    timestamppb.New(time.Unix(3600, 0)),
			HasStartChangepoint:          true,
			StartPositionLowerBound_99Th: 90,
			StartPositionUpperBound_99Th: 110,
			StartPositionDistribution:    model.SimpleDistribution(100, 10).Serialize(),
			EndPosition:                  400,
			EndHour:                      timestamppb.New(time.Unix(400*3600, 0)),
			FinalizedCounts: &cpb.Counts{
				TotalResults:             32, // 30 (existing) + 2 (positions 200 and 400)
				UnexpectedResults:        7,  // 5 (existing) + 2 (positions 200 and 400)
				ExpectedPassedResults:    1,
				ExpectedFailedResults:    2,
				ExpectedCrashedResults:   3,
				ExpectedAbortedResults:   4,
				UnexpectedPassedResults:  5,
				UnexpectedFailedResults:  6,
				UnexpectedCrashedResults: 9, // 7 (existing) + 2 (positions 200 and 400)
				UnexpectedAbortedResults: 8,

				TotalRuns:                22, // 20 (existing) + 2 (positions 200 and 400)
				UnexpectedUnretriedRuns:  4,  // 2 (existing) + 2 (positions 200 and 400)
				UnexpectedAfterRetryRuns: 3,
				FlakyRuns:                4,

				TotalSourceVerdicts:      12, // 10 (existing) + 2 (positions 200 and 400)
				UnexpectedSourceVerdicts: 2,  // 1 (existing) + 1 (position 400)
				FlakySourceVerdicts:      3,  // 2 (existing) + 1 (position 200)
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
		}
		assert.Loosely(t, len(tvb.FinalizedSegments.Segments), should.Equal(1))
		assert.Loosely(t, tvb.FinalizedSegments.Segments[0], should.Match(expectedFinalized))

		expectedFinalizing := &cpb.Segment{
			State:                        cpb.SegmentState_FINALIZING,
			StartPosition:                500,
			StartHour:                    timestamppb.New(time.Unix(500*3600, 0)),
			HasStartChangepoint:          true,
			StartPositionLowerBound_99Th: 490,
			StartPositionUpperBound_99Th: 510,
			StartPositionDistribution:    model.SimpleDistribution(500, 10).Serialize(),
			FinalizedCounts:              &cpb.Counts{},
		}

		assert.Loosely(t, tvb.FinalizingSegment, should.NotBeNil)
		assert.Loosely(t, tvb.FinalizingSegment, should.Match(expectedFinalizing))
	})

	ftt.Run("Should panic if no finalizing segment in evicted segments", t, func(t *ftt.Test) {
		tvb := Entry{
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
				StartPositionDistribution:    model.SimpleDistribution(100, 10).Serialize(),
				FinalizedCounts: &cpb.Counts{
					TotalResults:      30,
					UnexpectedResults: 5,

					TotalRuns:                20,
					UnexpectedUnretriedRuns:  2,
					UnexpectedAfterRetryRuns: 3,
					FlakyRuns:                4,

					TotalSourceVerdicts:      10,
					UnexpectedSourceVerdicts: 1,
					FlakySourceVerdicts:      2,
				},
				MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(7*3600, 0)),
			},
		}
		evictedSegments := []inputbuffer.EvictedSegment{
			{
				State:                          cpb.SegmentState_FINALIZED,
				StartPosition:                  200,
				StartHour:                      time.Unix(100*3600, 0),
				HasStartChangepoint:            false,
				StartPositionDistribution:      model.SimpleDistribution(200, 10),
				EndPosition:                    400,
				EndHour:                        time.Unix(400*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(10*3600, 0),
				// Verdicts are not relevant to this test.
				Runs: []*inputbuffer.Run{},
			},
		}
		f := func() { tvb.UpdateOutputBuffer(evictedSegments) }
		assert.Loosely(t, f, should.Panic)
	})

	ftt.Run("Statistics should be updated following eviction", t, func(t *ftt.Test) {
		tvb := Entry{
			Statistics: &cpb.Statistics{
				HourlyBuckets: []*cpb.Statistics_HourBucket{
					{
						Hour:                     999,
						UnexpectedSourceVerdicts: 1,
						FlakySourceVerdicts:      2,
						TotalSourceVerdicts:      3,
					},
					{
						Hour:                1000,
						FlakySourceVerdicts: 10,
						TotalSourceVerdicts: 10,
					},
				},
			},
		}
		evictedSegments := []inputbuffer.EvictedSegment{
			{
				State:                          cpb.SegmentState_FINALIZING,
				StartPosition:                  200,
				StartHour:                      time.Unix(100*3600, 0),
				HasStartChangepoint:            false,
				MostRecentUnexpectedResultHour: time.Unix(10*3600, 0),
				Runs: []*inputbuffer.Run{
					// Expected source verdict.
					{
						CommitPosition: 190,
						Hour:           time.Unix(1000*3600, 0),
						Expected:       inputbuffer.ResultCounts{PassCount: 1},
					},
					// Expected source verdict.
					{
						CommitPosition: 191,
						Hour:           time.Unix(1000*3600, 0),
						Expected:       inputbuffer.ResultCounts{PassCount: 2},
					},
					// Flaky source verdict.
					{
						CommitPosition: 192,
						Hour:           time.Unix(992*3600, 0),
						Expected: inputbuffer.ResultCounts{
							PassCount: 1,
						},
					},
					{
						CommitPosition: 192,
						Hour:           time.Unix(992*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
					// Unexpected source verdict which is too old for the retention policy.
					{
						CommitPosition: 193,
						Hour:           time.Unix(1*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
					{
						CommitPosition: 193,
						Hour:           time.Unix((1000-StatisticsRetentionDays*24)*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
					// Unexpected source verdict which is (just) within the retention policy.
					{
						CommitPosition: 194,
						Hour:           time.Unix(1*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
					{
						CommitPosition: 194,
						Hour:           time.Unix((1000-StatisticsRetentionDays*24+1)*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
					// Partial source verdict. While the runs we know about so far suggest
					// the verdict will be too old to be retained in a bucket, this can change
					// if a recent run is obtained at this commit position.
					{
						CommitPosition: 195,
						Hour:           time.Unix(10*3600, 0),
						Unexpected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
				},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)

		expected := &cpb.Statistics{
			HourlyBuckets: []*cpb.Statistics_HourBucket{
				{
					Hour:                     (1000 - StatisticsRetentionDays*24 + 1),
					UnexpectedSourceVerdicts: 1,
					TotalSourceVerdicts:      1,
				},
				{
					Hour:                992,
					FlakySourceVerdicts: 1,
					TotalSourceVerdicts: 1,
				},
				{
					Hour:                     999,
					UnexpectedSourceVerdicts: 1,
					FlakySourceVerdicts:      2,
					TotalSourceVerdicts:      3,
				},
				{
					Hour:                1000,
					FlakySourceVerdicts: 10,
					TotalSourceVerdicts: 12,
				},
			},
			PartialSourceVerdict: &cpb.PartialSourceVerdict{
				CommitPosition:    195,
				LastHour:          timestamppb.New(time.Unix(10*3600, 0)),
				UnexpectedResults: 1,
			},
		}
		assert.Loosely(t, tvb.Statistics, should.Match(expected))
		assert.Loosely(t, tvb.IsStatisticsDirty, should.BeTrue)
	})

	ftt.Run("Output buffer should not be updated if there is no eviction", t, func(t *ftt.Test) {
		tvb := Entry{}
		tvb.UpdateOutputBuffer([]inputbuffer.EvictedSegment{})
		assert.Loosely(t, tvb.IsStatisticsDirty, should.BeFalse)
		assert.Loosely(t, tvb.IsFinalizingSegmentDirty, should.BeFalse)
		assert.Loosely(t, tvb.IsFinalizedSegmentsDirty, should.BeFalse)
	})
}

func TestOutOfOrderRuns(t *testing.T) {
	ftt.Run("Test out of order runs", t, func(t *ftt.Test) {
		tvb := &Entry{
			IsNew:       true,
			Project:     "proj_3",
			TestID:      "test_id_3",
			VariantHash: "variant_hash_3",
			RefHash:     []byte("refhash3"),
			SourceRef: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host_3",
						Project: "proj_3",
						Ref:     "ref_3",
					},
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  100,
				HotBuffer:          inputbuffer.History{},
				ColdBufferCapacity: 2000,
				ColdBuffer:         inputbuffer.History{},
			},
		}

		t.Run("From empty", func(t *ftt.Test) {
			tvb.InputBuffer.ColdBuffer.Runs = nil
			tvb.InputBuffer.HotBuffer.Runs = nil
			tvb.FinalizingSegment = nil

			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 100}), should.BeTrue)
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 1}), should.BeTrue)
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 100}), should.BeTrue)
			assert.Loosely(t, tvb.InputBuffer.HotBuffer.Runs, should.HaveLength(3))
		})
		t.Run("With finalizing segment and cold buffer setting bound", func(t *ftt.Test) {
			tvb.InputBuffer.ColdBuffer.Runs = []inputbuffer.Run{
				{
					CommitPosition: 20,
					Hour:           time.Unix(0, 0),
					Expected: inputbuffer.ResultCounts{
						PassCount: 1,
					},
				},
			}
			tvb.FinalizingSegment = &cpb.Segment{
				State: cpb.SegmentState_FINALIZING,
			}
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 20}), should.BeTrue)
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 21}), should.BeTrue)
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 19}), should.BeFalse)
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 20}), should.BeTrue)
			assert.Loosely(t, tvb.InputBuffer.HotBuffer.Runs, should.HaveLength(3))
		})
		t.Run("With finalizing segment and hot buffer setting bound", func(t *ftt.Test) {
			tvb.InputBuffer.HotBuffer.Runs = []inputbuffer.Run{
				{
					CommitPosition: 20,
					Hour:           time.Unix(0, 0),
					Expected: inputbuffer.ResultCounts{
						PassCount: 1,
					},
				},
			}
			tvb.FinalizingSegment = &cpb.Segment{
				State: cpb.SegmentState_FINALIZING,
			}
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 20}), should.BeTrue)
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 21}), should.BeTrue)
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 19}), should.BeFalse)
			assert.Loosely(t, tvb.InsertToInputBuffer(inputbuffer.Run{CommitPosition: 20}), should.BeTrue)
			assert.Loosely(t, tvb.InputBuffer.HotBuffer.Runs, should.HaveLength(4))
		})
	})
}

func makeKey(proj string, testID string, variantHash string, refHash RefHash) Key {
	return Key{
		Project:     proj,
		TestID:      testID,
		VariantHash: variantHash,
		RefHash:     refHash,
	}
}

func BenchmarkEncodeSegments(b *testing.B) {
	// Result July 2024:
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkEncodeSegments-96    	    3024	    485035 ns/op	   18441 B/op	       2 allocs/op
	b.StopTimer()
	segs := testSegments()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := EncodeSegments(segs)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkDecodeSegments(b *testing.B) {
	// Result July 2024:
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkDecodeSegments-96    	    6175	    166155 ns/op	  118429 B/op	     611 allocs/op
	b.StopTimer()
	segs := testSegments()
	encodedSegs, err := EncodeSegments(segs)
	if err != nil {
		panic(err)
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := DecodeSegments(encodedSegs)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkEncodeStatistics(b *testing.B) {
	// Result when test added:
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkEncodeStatistics-96    	   15578	     71829 ns/op	    8321 B/op	       3 allocs/op
	b.StopTimer()
	stats := testStatistics()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := EncodeStatistics(stats)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkDecodeStatistics(b *testing.B) {
	// Result when test added:
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkDecodeStatistics-96    	   18616	     61122 ns/op	   34236 B/op	     276 allocs/op
	b.StopTimer()
	stats := testStatistics()
	encodedStats, err := EncodeStatistics(stats)
	if err != nil {
		panic(err)
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := DecodeStatistics(encodedStats)
		if err != nil {
			panic(err)
		}
	}
}

func testStatistics() *cpb.Statistics {
	var buckets []*cpb.Statistics_HourBucket
	for i := range StatisticsRetentionDays * 24 {
		buckets = append(buckets, &cpb.Statistics_HourBucket{
			Hour:                     int64(i),
			UnexpectedSourceVerdicts: int64(i + 1),
			FlakySourceVerdicts:      int64(i + 2),
			TotalSourceVerdicts:      int64(2*i + 3),
		})
	}
	return &cpb.Statistics{HourlyBuckets: buckets}
}

func testSegments() *cpb.Segments {
	var segments []*cpb.Segment
	for i := range 100 {
		segments = append(segments, &cpb.Segment{
			State:                          cpb.SegmentState_FINALIZED,
			HasStartChangepoint:            true,
			StartPosition:                  int64(162*i + 5),
			StartPositionLowerBound_99Th:   int64(162 * i),
			StartPositionUpperBound_99Th:   int64(162*i + 10),
			EndPosition:                    int64(162 * (i + 1)),
			StartHour:                      timestamppb.New(time.Unix(int64(i*12*3600), 0)),
			EndHour:                        timestamppb.New(time.Unix(int64((i+1)*12*3600), 0)),
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(int64((i+1)*9*3600), 0)),
			FinalizedCounts: &cpb.Counts{
				UnexpectedResults: int64(i),
				TotalResults:      int64(i + 1),

				UnexpectedUnretriedRuns:  int64(i + 2),
				UnexpectedAfterRetryRuns: int64(i + 3),
				FlakyRuns:                int64(i + 4),
				TotalRuns:                int64(4*i + 5),

				UnexpectedSourceVerdicts: int64(i + 6),
				FlakySourceVerdicts:      int64(i + 7),
				TotalSourceVerdicts:      int64(2*i + 8),
			},
			StartPositionDistribution: model.SimpleDistribution(int64(162*i), 5).Serialize(),
		})
	}
	return &cpb.Segments{
		Segments: segments,
	}
}
