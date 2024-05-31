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

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	tu "go.chromium.org/luci/analysis/internal/changepoints/testutil"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetchUpdateTestVariantBranch(t *testing.T) {
	Convey("Fetch not found", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		key := Key{
			Project:     "proj",
			TestID:      "test_id",
			VariantHash: "variant_hash",
			RefHash:     "git_hash",
		}
		tvbs, err := Read(span.Single(ctx), []Key{key})
		So(err, ShouldBeNil)
		So(len(tvbs), ShouldEqual, 1)
		So(tvbs[0], ShouldBeNil)
	})

	Convey("Insert and fetch", t, func() {
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
					Verdicts: []inputbuffer.PositionVerdict{
						{
							CommitPosition:       15,
							IsSimpleExpectedPass: true,
							Hour:                 time.Unix(0, 0),
						},
						{
							CommitPosition:       18,
							IsSimpleExpectedPass: false,
							Hour:                 time.Unix(0, 0),
							Details: inputbuffer.VerdictDetails{
								IsExonerated: true,
								Runs: []inputbuffer.Run{
									{
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
					Verdicts: []inputbuffer.PositionVerdict{
						{
							CommitPosition:       20,
							IsSimpleExpectedPass: true,
							Hour:                 time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
		}

		var hs inputbuffer.HistorySerializer
		mutation1, err := tvb1.ToMutation(&hs)
		So(err, ShouldBeNil)
		mutation3, err := tvb3.ToMutation(&hs)
		So(err, ShouldBeNil)
		testutil.MustApply(ctx, mutation1, mutation3)

		tvbks := []Key{
			makeKey("proj_1", "test_id_1", "variant_hash_1", "refhash1"),
			makeKey("proj_2", "test_id_2", "variant_hash_2", "refhash2"),
			makeKey("proj_3", "test_id_3", "variant_hash_3", "refhash3"),
		}
		tvbs, err := Read(span.Single(ctx), tvbks)
		So(err, ShouldBeNil)
		So(len(tvbs), ShouldEqual, 3)
		// After inserting, the record should not be new anymore.
		tvb1.IsNew = false
		// After decoding, cold buffer should be empty.
		tvb1.InputBuffer.ColdBuffer = inputbuffer.History{Verdicts: []inputbuffer.PositionVerdict{}}

		So(tvbs[0], ShouldResembleProto, tvb1)

		So(tvbs[1], ShouldBeNil)

		tvb3.IsNew = false
		tvb3.InputBuffer.ColdBuffer = inputbuffer.History{Verdicts: []inputbuffer.PositionVerdict{}}

		So(tvbs[2], ShouldResembleProto, tvb3)
	})

	Convey("Insert and update", t, func() {
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
					Verdicts: []inputbuffer.PositionVerdict{
						{
							CommitPosition:       15,
							IsSimpleExpectedPass: true,
							Hour:                 time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
		}

		var hs inputbuffer.HistorySerializer
		mutation, err := tvb.ToMutation(&hs)
		So(err, ShouldBeNil)
		testutil.MustApply(ctx, mutation)

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
					Verdicts: []inputbuffer.PositionVerdict{
						{
							CommitPosition:       16,
							IsSimpleExpectedPass: true,
							Hour:                 time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
				ColdBuffer: inputbuffer.History{
					Verdicts: []inputbuffer.PositionVerdict{
						{
							CommitPosition:       15,
							IsSimpleExpectedPass: false,
							Hour:                 time.Unix(0, 0),
							Details: inputbuffer.VerdictDetails{
								IsExonerated: false,
								Runs: []inputbuffer.Run{
									{
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
						},
					},
				},
			},
			IsFinalizedSegmentsDirty: true,
			Statistics: &cpb.Statistics{
				HourlyBuckets: []*cpb.Statistics_HourBucket{{
					Hour:               100,
					UnexpectedVerdicts: 1,
					FlakyVerdicts:      2,
					TotalVerdicts:      4,
				}},
			},
			IsStatisticsDirty: true,
		}

		mutation, err = tvb.ToMutation(&hs)
		So(err, ShouldBeNil)
		testutil.MustApply(ctx, mutation)

		tvbks := []Key{
			makeKey("proj_1", "test_id_1", "variant_hash_1", "githash1"),
		}
		tvbs, err := Read(span.Single(ctx), tvbks)
		So(err, ShouldBeNil)
		So(len(tvbs), ShouldEqual, 1)

		tvb.IsStatisticsDirty = false
		tvb.IsFinalizedSegmentsDirty = false
		tvb.IsFinalizingSegmentDirty = false
		tvb.InputBuffer.IsColdBufferDirty = false

		So(tvbs[0], ShouldResembleProto, tvb)
	})
}

func TestInsertToInputBuffer(t *testing.T) {
	Convey("Insert simple test variant", t, func() {
		tvb := &Entry{
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  10,
				ColdBufferCapacity: 100,
			},
		}
		payload := tu.SamplePayload()
		sourcesMap := tu.SampleSourcesMap(12)
		tv := &rdbpb.TestVariant{
			Status: rdbpb.TestVariantStatus_EXPECTED,
			Results: []*rdbpb.TestResultBundle{
				{
					Result: &rdbpb.TestResult{
						Expected: true,
						Status:   rdbpb.TestStatus_PASS,
					},
				},
			},
			SourcesId: "sources_id",
		}
		pv, err := ToPositionVerdict(tv, payload, map[string]bool{}, sourcesMap["sources_id"])
		So(err, ShouldBeNil)
		tvb.InsertToInputBuffer(pv)
		So(len(tvb.InputBuffer.HotBuffer.Verdicts), ShouldEqual, 1)

		So(tvb.InputBuffer.HotBuffer.Verdicts[0], ShouldResemble, inputbuffer.PositionVerdict{
			CommitPosition:       12,
			IsSimpleExpectedPass: true,
			Hour:                 payload.PartitionTime.AsTime(),
		})
	})

	Convey("Insert non-simple test variant", t, func() {
		tvb := &Entry{
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  10,
				ColdBufferCapacity: 100,
			},
		}
		payload := tu.SamplePayload()
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
			},
		}
		duplicateMap := map[string]bool{
			"run-1": true,
			"run-3": true,
		}
		pv, err := ToPositionVerdict(tv, payload, duplicateMap, sourcesMap["sources_id"])
		So(err, ShouldBeNil)
		tvb.InsertToInputBuffer(pv)
		So(len(tvb.InputBuffer.HotBuffer.Verdicts), ShouldEqual, 1)

		So(tvb.InputBuffer.HotBuffer.Verdicts[0], ShouldResemble, inputbuffer.PositionVerdict{
			CommitPosition:       12,
			IsSimpleExpectedPass: false,
			Hour:                 payload.PartitionTime.AsTime(),
			Details: inputbuffer.VerdictDetails{
				IsExonerated: false,
				Runs: []inputbuffer.Run{
					{
						Unexpected: inputbuffer.ResultCounts{
							CrashCount: 1,
							AbortCount: 1,
						},
						IsDuplicate: false,
					},
					{
						Expected: inputbuffer.ResultCounts{
							AbortCount: 1,
						},
						IsDuplicate: false,
					},
					{
						Unexpected: inputbuffer.ResultCounts{
							PassCount: 1,
							FailCount: 1,
						},
						IsDuplicate: true,
					},
					{
						Expected: inputbuffer.ResultCounts{
							PassCount:  1,
							FailCount:  1,
							CrashCount: 1,
						},
						IsDuplicate: true,
					},
				},
			},
		})
	})
}

func TestUpdateOutputBuffer(t *testing.T) {
	Convey("No existing finalizing segment", t, func() {
		tvb := Entry{}
		evictedSegments := []inputbuffer.EvictedSegment{
			{
				State:         cpb.SegmentState_FINALIZED,
				StartPosition: 1,
				StartHour:     time.Unix(1*3600, 0),
				EndPosition:   10,
				EndHour:       time.Unix(10*3600, 0),
				Verdicts: []inputbuffer.PositionVerdict{
					{
						CommitPosition: 2,
						Hour:           time.Unix(10*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
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
					},
				},
				MostRecentUnexpectedResultHour: time.Unix(7*3600, 0),
			},
			{
				State:                          cpb.SegmentState_FINALIZING,
				HasStartChangepoint:            true,
				StartPosition:                  11,
				StartHour:                      time.Unix(11*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix((100000-StatisticsRetentionDays*24)*3600, 0),
				Verdicts: []inputbuffer.PositionVerdict{
					{
						CommitPosition:       11,
						Hour:                 time.Unix(11*3600, 0),
						IsSimpleExpectedPass: true,
					},
					{
						CommitPosition:       12,
						Hour:                 time.Unix((100000-StatisticsRetentionDays*24)*3600, 0),
						IsSimpleExpectedPass: true,
					},
					{
						CommitPosition:       13,
						Hour:                 time.Unix((100000-StatisticsRetentionDays*24+1)*3600, 0),
						IsSimpleExpectedPass: false,
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Expected: inputbuffer.ResultCounts{
										PassCount: 1,
									},
									Unexpected: inputbuffer.ResultCounts{
										FailCount: 1,
									},
								},
							},
						},
					},
					{
						CommitPosition:       11,
						Hour:                 time.Unix(100000*3600, 0),
						IsSimpleExpectedPass: false,
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Unexpected: inputbuffer.ResultCounts{
										CrashCount: 1,
									},
								},
							},
						},
					},
				},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)

		So(tvb.FinalizingSegment, ShouldResembleProto, &cpb.Segment{
			State:               cpb.SegmentState_FINALIZING,
			HasStartChangepoint: true,
			StartPosition:       11,
			StartHour:           timestamppb.New(time.Unix(11*3600, 0)),
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

				TotalVerdicts:      4,
				FlakyVerdicts:      1,
				UnexpectedVerdicts: 1,
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix((100000-StatisticsRetentionDays*24)*3600, 0)),
		})
		So(tvb.IsFinalizingSegmentDirty, ShouldBeTrue)

		So(len(tvb.FinalizedSegments.Segments), ShouldEqual, 1)
		So(tvb.FinalizedSegments.Segments[0], ShouldResembleProto, &cpb.Segment{
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

				TotalVerdicts: 1,
				FlakyVerdicts: 1,
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(7*3600, 0)),
		})
		So(tvb.IsFinalizedSegmentsDirty, ShouldBeTrue)

		So(tvb.Statistics, ShouldResembleProto, &cpb.Statistics{
			HourlyBuckets: []*cpb.Statistics_HourBucket{
				// Confirm that buckets for hours 11 and (100000 - StatisticsRetentionDays*24)
				// are not present due to retention policies.
				{
					Hour:          (100000 - StatisticsRetentionDays*24 + 1),
					TotalVerdicts: 1,
					FlakyVerdicts: 1,
				},
				{
					Hour:               100000,
					TotalVerdicts:      1,
					UnexpectedVerdicts: 1,
				},
			},
		})
		So(tvb.IsStatisticsDirty, ShouldBeTrue)
	})

	Convey("Combine finalizing segment with finalizing segment", t, func() {
		tvb := Entry{
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
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

					TotalVerdicts:      10,
					UnexpectedVerdicts: 1,
					FlakyVerdicts:      2,
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
				StartPositionLowerBound99Th:    190,
				StartPositionUpperBound99Th:    210,
				MostRecentUnexpectedResultHour: time.Unix(10*3600, 0),
				Verdicts: []inputbuffer.PositionVerdict{
					{
						CommitPosition:       200,
						Hour:                 time.Unix(5*3600, 0),
						IsSimpleExpectedPass: false,
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
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
							},
						},
					},
					{
						CommitPosition: 250,
						Hour:           time.Unix(15*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Expected: inputbuffer.ResultCounts{
										FailCount: 1,
									},
								},
							},
						},
					},
				},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)
		So(tvb.FinalizedSegments, ShouldBeNil)
		So(tvb.FinalizingSegment, ShouldNotBeNil)

		expected := &cpb.Segment{
			State:                        cpb.SegmentState_FINALIZING,
			StartPosition:                100,
			StartHour:                    timestamppb.New(time.Unix(3600, 0)),
			HasStartChangepoint:          true,
			StartPositionLowerBound_99Th: 90,
			StartPositionUpperBound_99Th: 110,
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

				TotalVerdicts:      12, // two more, for positions 200 and 250.
				UnexpectedVerdicts: 1,
				FlakyVerdicts:      3, // one more, for commit position 200.
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
		}
		So(tvb.FinalizingSegment, ShouldResembleProto, expected)
	})

	Convey("Combine finalizing segment with finalized segment", t, func() {
		tvb := Entry{
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
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

					TotalVerdicts:      10,
					UnexpectedVerdicts: 1,
					FlakyVerdicts:      2,
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
				StartPositionLowerBound99Th:    190,
				StartPositionUpperBound99Th:    210,
				EndPosition:                    400,
				EndHour:                        time.Unix(400*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(10*3600, 0),
				Verdicts: []inputbuffer.PositionVerdict{
					{
						CommitPosition:       200,
						Hour:                 time.Unix(5*3600, 0),
						IsSimpleExpectedPass: false,
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
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
							},
						},
					},
					{
						CommitPosition: 250,
						Hour:           time.Unix(15*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Expected: inputbuffer.ResultCounts{
										FailCount: 1,
									},
								},
							},
						},
					},
				},
			},
			{
				State:                       cpb.SegmentState_FINALIZING,
				HasStartChangepoint:         true,
				StartPosition:               500,
				StartHour:                   time.Unix(20*3600, 0),
				StartPositionLowerBound99Th: 490,
				StartPositionUpperBound99Th: 510,
				EndPosition:                 800,
				Verdicts: []inputbuffer.PositionVerdict{
					{
						CommitPosition: 500,
						Hour:           time.Unix(20*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Unexpected: inputbuffer.ResultCounts{
										CrashCount: 2,
									},
								},
							},
						},
					},
					{
						CommitPosition: 510,
						Hour:           time.Unix(25*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Unexpected: inputbuffer.ResultCounts{
										CrashCount: 4,
									},
								},
							},
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

				TotalVerdicts:      12, // two more, for commit position 200 and 250.
				UnexpectedVerdicts: 1,
				FlakyVerdicts:      3, // one more, for commit position 200.
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
		}
		So(len(tvb.FinalizedSegments.Segments), ShouldEqual, 1)
		So(tvb.FinalizedSegments.Segments[0], ShouldResembleProto, expectedFinalizedSegment)

		expectedFinalizingSegment := &cpb.Segment{
			State:                        cpb.SegmentState_FINALIZING,
			HasStartChangepoint:          true,
			StartPosition:                500,
			StartHour:                    timestamppb.New(time.Unix(20*3600, 0)),
			StartPositionLowerBound_99Th: 490,
			StartPositionUpperBound_99Th: 510,
			FinalizedCounts: &cpb.Counts{
				TotalResults:             6,
				UnexpectedResults:        6,
				UnexpectedCrashedResults: 6,

				TotalRuns:                2,
				UnexpectedAfterRetryRuns: 2,

				TotalVerdicts:      2,
				UnexpectedVerdicts: 2,
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(25*3600, 0)),
		}
		So(tvb.FinalizingSegment, ShouldResembleProto, expectedFinalizingSegment)
	})

	Convey("Combine finalizing segment with finalized segment, with a token of finalizing segment in input buffer", t, func() {
		tvb := Entry{
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
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

					TotalVerdicts:      10,
					UnexpectedVerdicts: 1,
					FlakyVerdicts:      2,
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
				Verdicts: []inputbuffer.PositionVerdict{
					{
						CommitPosition: 200,
						Hour:           time.Unix(100*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Unexpected: inputbuffer.ResultCounts{
										CrashCount: 1,
									},
								},
							},
						},
					},
					{
						CommitPosition: 400,
						Hour:           time.Unix(90*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Unexpected: inputbuffer.ResultCounts{
										CrashCount: 1,
									},
								},
							},
						},
					},
				},
			},
			{
				State:                       cpb.SegmentState_FINALIZING,
				StartPosition:               500,
				StartHour:                   time.Unix(500*3600, 0),
				HasStartChangepoint:         true,
				StartPositionLowerBound99Th: 490,
				StartPositionUpperBound99Th: 510,
				Verdicts:                    []inputbuffer.PositionVerdict{},
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

				TotalVerdicts:      12, // 10 (existing) + 2 (positions 200 and 400)
				UnexpectedVerdicts: 3,  // 1 (existing) + 2 (position 200 and 400)
				FlakyVerdicts:      2,  // 2 (existing)
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
		}
		So(len(tvb.FinalizedSegments.Segments), ShouldEqual, 1)
		So(tvb.FinalizedSegments.Segments[0], ShouldResembleProto, expectedFinalized)

		expectedFinalizing := &cpb.Segment{
			State:                        cpb.SegmentState_FINALIZING,
			StartPosition:                500,
			StartHour:                    timestamppb.New(time.Unix(500*3600, 0)),
			HasStartChangepoint:          true,
			StartPositionLowerBound_99Th: 490,
			StartPositionUpperBound_99Th: 510,
			FinalizedCounts:              &cpb.Counts{},
		}

		So(tvb.FinalizingSegment, ShouldNotBeNil)
		So(tvb.FinalizingSegment, ShouldResembleProto, expectedFinalizing)
	})

	Convey("Should panic if no finalizing segment in evicted segments", t, func() {
		tvb := Entry{
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
				FinalizedCounts: &cpb.Counts{
					TotalResults:             30,
					UnexpectedResults:        5,
					TotalRuns:                20,
					UnexpectedUnretriedRuns:  2,
					UnexpectedAfterRetryRuns: 3,
					FlakyRuns:                4,
					TotalVerdicts:            10,
					UnexpectedVerdicts:       1,
					FlakyVerdicts:            2,
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
				StartPositionLowerBound99Th:    190,
				StartPositionUpperBound99Th:    210,
				EndPosition:                    400,
				EndHour:                        time.Unix(400*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(10*3600, 0),
				Verdicts:                       []inputbuffer.PositionVerdict{},
			},
		}
		f := func() { tvb.UpdateOutputBuffer(evictedSegments) }
		So(f, ShouldPanic)
	})

	Convey("Statistics should be updated following eviction", t, func() {
		tvb := Entry{
			Statistics: &cpb.Statistics{
				HourlyBuckets: []*cpb.Statistics_HourBucket{
					{
						Hour:               999,
						UnexpectedVerdicts: 1,
						FlakyVerdicts:      2,
						TotalVerdicts:      3,
					},
					{
						Hour:          1000,
						FlakyVerdicts: 10,
						TotalVerdicts: 10,
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
				Verdicts: []inputbuffer.PositionVerdict{
					// Expected verdict.
					{
						CommitPosition:       190,
						Hour:                 time.Unix(1000*3600, 0),
						IsSimpleExpectedPass: true,
					},
					// Expected verdict.
					{
						CommitPosition: 191,
						Hour:           time.Unix(1000*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Expected: inputbuffer.ResultCounts{
										PassCount: 2,
									},
								},
							},
						},
					},
					// Flaky verdict.
					{
						CommitPosition: 192,
						Hour:           time.Unix(992*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Expected: inputbuffer.ResultCounts{
										PassCount: 1,
									},
								},
								{
									IsDuplicate: true,
									Unexpected: inputbuffer.ResultCounts{
										FailCount: 1,
									},
								},
							},
						},
					},
					// Unexpected verdict.
					{
						CommitPosition: 193,
						Hour:           time.Unix(991*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Unexpected: inputbuffer.ResultCounts{
										FailCount: 1,
									},
								},
								{
									Unexpected: inputbuffer.ResultCounts{
										FailCount: 1,
									},
								},
							},
						},
					},
					// Verdict which is (just) within the retention policy.
					{
						CommitPosition: 194,
						Hour:           time.Unix((1000-StatisticsRetentionDays*24+1)*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Unexpected: inputbuffer.ResultCounts{
										FailCount: 1,
									},
								},
								{
									Unexpected: inputbuffer.ResultCounts{
										FailCount: 1,
									},
								},
							},
						},
					},
					// Verdict which is from too old a bucket.
					{
						CommitPosition: 194,
						Hour:           time.Unix((1000-StatisticsRetentionDays*24)*3600, 0),
						Details: inputbuffer.VerdictDetails{
							Runs: []inputbuffer.Run{
								{
									Unexpected: inputbuffer.ResultCounts{
										FailCount: 1,
									},
								},
							},
						},
					},
				},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)

		expected := &cpb.Statistics{
			HourlyBuckets: []*cpb.Statistics_HourBucket{
				{
					Hour:               (1000 - StatisticsRetentionDays*24 + 1),
					UnexpectedVerdicts: 1,
					TotalVerdicts:      1,
				},
				{
					Hour:               991,
					UnexpectedVerdicts: 1,
					TotalVerdicts:      1,
				},
				{
					Hour:          992,
					FlakyVerdicts: 1,
					TotalVerdicts: 1,
				},
				{
					Hour:               999,
					UnexpectedVerdicts: 1,
					FlakyVerdicts:      2,
					TotalVerdicts:      3,
				},
				{
					Hour:          1000,
					FlakyVerdicts: 10,
					TotalVerdicts: 12,
				},
			},
		}
		So(tvb.Statistics, ShouldResembleProto, expected)
		So(tvb.IsStatisticsDirty, ShouldBeTrue)
	})

	Convey("Output buffer should not be updated if there is no eviction", t, func() {
		tvb := Entry{}
		tvb.UpdateOutputBuffer([]inputbuffer.EvictedSegment{})
		So(tvb.IsStatisticsDirty, ShouldBeFalse)
		So(tvb.IsFinalizingSegmentDirty, ShouldBeFalse)
		So(tvb.IsFinalizedSegmentsDirty, ShouldBeFalse)
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
	// Result when test added:
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkEncodeSegments-96    	    3343	    328663 ns/op	   14598 B/op	       3 allocs/op
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
	// Result when test added:
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkDecodeSegments-96    	   10663	    111653 ns/op	   53190 B/op	     510 allocs/op
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
	for i := 0; i < StatisticsRetentionDays*24; i++ {
		buckets = append(buckets, &cpb.Statistics_HourBucket{
			Hour:               int64(i),
			UnexpectedVerdicts: int64(i + 1),
			FlakyVerdicts:      int64(i + 2),
			TotalVerdicts:      int64(2*i + 3),
		})
	}
	return &cpb.Statistics{HourlyBuckets: buckets}
}

func testSegments() *cpb.Segments {
	var segments []*cpb.Segment
	for i := 0; i < 100; i++ {
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

				UnexpectedVerdicts: int64(i + 6),
				FlakyVerdicts:      int64(i + 7),
				TotalVerdicts:      int64(2*i + 8),
			},
		})
	}
	return &cpb.Segments{
		Segments: segments,
	}
}
