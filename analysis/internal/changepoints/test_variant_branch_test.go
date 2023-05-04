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

package changepoints

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/span"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	changepointspb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/testutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestFetchUpdateTestVariantBranch(t *testing.T) {
	Convey("Fetch not found", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		key := TestVariantBranchKey{
			Project:     "proj",
			TestID:      "test_id",
			VariantHash: "variant_hash",
			RefHash:     "git_hash",
		}
		tvbs, err := ReadTestVariantBranches(span.Single(ctx), []TestVariantBranchKey{key})
		So(err, ShouldBeNil)
		So(len(tvbs), ShouldEqual, 1)
		So(tvbs[0], ShouldBeNil)
	})

	Convey("Insert and fetch", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		tvb1 := &TestVariantBranch{
			IsNew:       true,
			Project:     "proj_1",
			TestID:      "test_id_1",
			VariantHash: "variant_hash_1",
			RefHash:     []byte("refhash1"),
			Variant: &analysispb.Variant{
				Def: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			SourceRef: &analysispb.SourceRef{
				System: &analysispb.SourceRef_Gitiles{
					Gitiles: &analysispb.GitilesRef{
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
							CommitPosition:   15,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
			RecentChangepointCount: 0,
		}

		tvb3 := &TestVariantBranch{
			IsNew:       true,
			Project:     "proj_3",
			TestID:      "test_id_3",
			VariantHash: "variant_hash_3",
			RefHash:     []byte("refhash3"),
			SourceRef: &analysispb.SourceRef{
				System: &analysispb.SourceRef_Gitiles{
					Gitiles: &analysispb.GitilesRef{
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
							CommitPosition:   20,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
			RecentChangepointCount: 0,
		}

		mutation1, err := tvb1.ToMutation()
		So(err, ShouldBeNil)
		mutation3, err := tvb3.ToMutation()
		So(err, ShouldBeNil)
		testutil.MustApply(ctx, mutation1, mutation3)

		tvbks := []TestVariantBranchKey{
			makeTestVariantBranchKey("proj_1", "test_id_1", "variant_hash_1", "refhash1"),
			makeTestVariantBranchKey("proj_2", "test_id_2", "variant_hash_2", "refhash2"),
			makeTestVariantBranchKey("proj_3", "test_id_3", "variant_hash_3", "refhash3"),
		}
		tvbs, err := ReadTestVariantBranches(span.Single(ctx), tvbks)
		So(err, ShouldBeNil)
		So(len(tvbs), ShouldEqual, 3)
		// After inserting, the record should not be new anymore.
		tvb1.IsNew = false
		// After decoding, cold buffer should be empty.
		tvb1.InputBuffer.ColdBuffer = inputbuffer.History{Verdicts: []inputbuffer.PositionVerdict{}}
		// Compare the protobuf separately, as ShouldResemble does not work.
		sourceRef1 := tvbs[0].SourceRef
		sourceRef2 := tvb1.SourceRef
		tvbs[0].SourceRef = nil
		tvb1.SourceRef = nil

		So(tvbs[0], ShouldResemble, tvb1)
		So(tvbs[1], ShouldBeNil)
		So(sourceRef1, ShouldResembleProto, sourceRef2)

		tvb3.IsNew = false
		tvb3.InputBuffer.ColdBuffer = inputbuffer.History{Verdicts: []inputbuffer.PositionVerdict{}}
		// Compare the protobuf separately, as ShouldResemble does not work.
		sourceRef1 = tvbs[2].SourceRef
		sourceRef2 = tvb3.SourceRef
		tvbs[2].SourceRef = nil
		tvb3.SourceRef = nil

		So(tvbs[2], ShouldResemble, tvb3)
		So(sourceRef1, ShouldResembleProto, sourceRef2)
	})

	Convey("Insert and update", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		// Insert a new record.
		tvb := &TestVariantBranch{
			IsNew:       true,
			Project:     "proj_1",
			TestID:      "test_id_1",
			VariantHash: "variant_hash_1",
			RefHash:     []byte("githash1"),
			Variant: &analysispb.Variant{
				Def: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			SourceRef: &analysispb.SourceRef{
				System: &analysispb.SourceRef_Gitiles{
					Gitiles: &analysispb.GitilesRef{
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
							CommitPosition:   15,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
			RecentChangepointCount: 0,
		}

		mutation, err := tvb.ToMutation()
		So(err, ShouldBeNil)
		testutil.MustApply(ctx, mutation)

		// Update the record
		tvb = &TestVariantBranch{
			Project:     "proj_1",
			TestID:      "test_id_1",
			VariantHash: "variant_hash_1",
			RefHash:     []byte("githash1"),
			Variant: &analysispb.Variant{
				Def: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			SourceRef: &analysispb.SourceRef{
				System: &analysispb.SourceRef_Gitiles{
					Gitiles: &analysispb.GitilesRef{
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
							CommitPosition:   16,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
				ColdBuffer: inputbuffer.History{
					Verdicts: []inputbuffer.PositionVerdict{
						{
							CommitPosition:   15,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				IsColdBufferDirty: true,
			},
			RecentChangepointCount: 0,
			FinalizingSegment: &changepointspb.Segment{
				State:                        changepointspb.SegmentState_FINALIZING,
				HasStartChangepoint:          true,
				StartPosition:                50,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				StartPositionLowerBound_99Th: 45,
				StartPositionUpperBound_99Th: 55,
				FinalizedCounts: &changepointspb.Counts{
					TotalResults: 10,
					TotalRuns:    10,
					FlakyRuns:    10,
				},
			},
			FinalizedSegments: &changepointspb.Segments{
				Segments: []*changepointspb.Segment{
					{
						State:                        changepointspb.SegmentState_FINALIZED,
						HasStartChangepoint:          true,
						StartPosition:                20,
						StartHour:                    timestamppb.New(time.Unix(3600, 0)),
						StartPositionLowerBound_99Th: 10,
						StartPositionUpperBound_99Th: 30,
						EndPosition:                  40,
						EndHour:                      timestamppb.New(time.Unix(3600, 0)),
						FinalizedCounts: &changepointspb.Counts{
							TotalResults: 10,
							TotalRuns:    10,
							FlakyRuns:    10,
						},
					},
				},
			},
			IsFinalizingSegmentDirty: true,
			IsFinalizedSegmentsDirty: true,
		}

		mutation, err = tvb.ToMutation()
		So(err, ShouldBeNil)
		testutil.MustApply(ctx, mutation)

		tvbks := []TestVariantBranchKey{
			makeTestVariantBranchKey("proj_1", "test_id_1", "variant_hash_1", "githash1"),
		}
		tvbs, err := ReadTestVariantBranches(span.Single(ctx), tvbks)
		So(err, ShouldBeNil)
		So(len(tvbs), ShouldEqual, 1)

		// We cannot use ShouldResemble to compare proto messages. We will compare
		// them separately.
		tvb.IsNew = false
		tvb.InputBuffer.IsColdBufferDirty = false
		tvb.IsFinalizedSegmentsDirty = false
		tvb.IsFinalizingSegmentDirty = false

		// Captures finalizing segment in separate variables.
		finalizingSegment1 := tvb.FinalizingSegment
		finalizingSegment2 := tvbs[0].FinalizingSegment
		tvb.FinalizingSegment = nil
		tvbs[0].FinalizingSegment = nil

		// Captures finalized segments in separate variables.
		finalizedSegments1 := tvb.FinalizedSegments
		finalizedSegments2 := tvbs[0].FinalizedSegments
		tvb.FinalizedSegments = nil
		tvbs[0].FinalizedSegments = nil

		// Captures finalized segments in separate variables.
		sourceRef1 := tvb.SourceRef
		sourceRef2 := tvbs[0].SourceRef
		tvb.SourceRef = nil
		tvbs[0].SourceRef = nil

		So(tvbs[0], ShouldResemble, tvb)
		So(finalizingSegment1, ShouldResembleProto, finalizingSegment2)
		So(finalizedSegments1, ShouldResembleProto, finalizedSegments2)
		So(sourceRef1, ShouldResembleProto, sourceRef2)
	})
}

func TestInsertToInputBuffer(t *testing.T) {
	Convey("Insert simple test variant", t, func() {
		tvb := &TestVariantBranch{
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  10,
				ColdBufferCapacity: 100,
			},
		}
		payload := samplePayload(12)
		tv := &rdbpb.TestVariant{
			Status: rdbpb.TestVariantStatus_EXPECTED,
			Results: []*rdbpb.TestResultBundle{
				{
					Result: &rdbpb.TestResult{
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*10, 0)),
					},
				},
			},
		}
		pv, err := toPositionVerdict(tv, payload, map[string]bool{})
		So(err, ShouldBeNil)
		tvb.InsertToInputBuffer(pv)
		So(len(tvb.InputBuffer.HotBuffer.Verdicts), ShouldEqual, 1)

		So(tvb.InputBuffer.HotBuffer.Verdicts[0], ShouldResemble, inputbuffer.PositionVerdict{
			CommitPosition:   12,
			IsSimpleExpected: true,
			Hour:             tv.Results[0].Result.StartTime.AsTime(),
		})
	})

	Convey("Insert non-simple test variant", t, func() {
		tvb := &TestVariantBranch{
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  10,
				ColdBufferCapacity: 100,
			},
		}
		payload := samplePayload(12)
		tv := &rdbpb.TestVariant{
			Status: rdbpb.TestVariantStatus_FLAKY,
			Results: []*rdbpb.TestResultBundle{
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-1/tests/abc",
						Expected:  false,
						StartTime: timestamppb.New(time.Unix(3600*10, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-1/tests/abc",
						Expected:  false,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-1/tests/abc",
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-2/tests/abc",
						Expected:  false,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-3/tests/abc",
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-3/tests/abc",
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-4/tests/abc",
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
			},
		}
		duplicateMap := map[string]bool{
			"run-1": true,
			"run-3": true,
		}
		pv, err := toPositionVerdict(tv, payload, duplicateMap)
		So(err, ShouldBeNil)
		tvb.InsertToInputBuffer(pv)
		So(len(tvb.InputBuffer.HotBuffer.Verdicts), ShouldEqual, 1)

		So(tvb.InputBuffer.HotBuffer.Verdicts[0], ShouldResemble, inputbuffer.PositionVerdict{
			CommitPosition:   12,
			IsSimpleExpected: false,
			Hour:             tv.Results[0].Result.StartTime.AsTime(),
			Details: inputbuffer.VerdictDetails{
				IsExonerated: false,
				Runs: []inputbuffer.Run{
					{
						ExpectedResultCount:   0,
						UnexpectedResultCount: 1,
						IsDuplicate:           false,
					},
					{
						ExpectedResultCount:   1,
						UnexpectedResultCount: 0,
						IsDuplicate:           false,
					},
					{
						ExpectedResultCount:   1,
						UnexpectedResultCount: 2,
						IsDuplicate:           true,
					},
					{
						ExpectedResultCount:   2,
						UnexpectedResultCount: 0,
						IsDuplicate:           true,
					},
				},
			},
		})
	})
}

func TestUpdateOutputBuffer(t *testing.T) {
	Convey("No existing finalizing segment", t, func() {
		tvb := TestVariantBranch{}
		evictedSegments := []*changepointspb.Segment{
			{
				State:         changepointspb.SegmentState_FINALIZED,
				StartPosition: 1,
				EndPosition:   10,
				FinalizedCounts: &changepointspb.Counts{
					TotalResults:  10,
					TotalRuns:     10,
					TotalVerdicts: 10,
				},
			},
			{
				State:         changepointspb.SegmentState_FINALIZING,
				StartPosition: 11,
				EndPosition:   30,
				FinalizedCounts: &changepointspb.Counts{
					TotalResults:  20,
					TotalRuns:     20,
					TotalVerdicts: 20,
				},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)
		So(len(tvb.FinalizedSegments.Segments), ShouldEqual, 1)
		So(tvb.FinalizingSegment, ShouldNotBeNil)
		So(tvb.FinalizingSegment, ShouldResembleProto, evictedSegments[1])
		So(tvb.FinalizedSegments.Segments[0], ShouldResembleProto, evictedSegments[0])
	})

	Convey("Combine finalizing segment with finalizing segment", t, func() {
		tvb := TestVariantBranch{
			FinalizingSegment: &changepointspb.Segment{
				State:                        changepointspb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
				FinalizedCounts: &changepointspb.Counts{
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
		evictedSegments := []*changepointspb.Segment{
			{
				State:                        changepointspb.SegmentState_FINALIZING,
				StartPosition:                200,
				StartHour:                    timestamppb.New(time.Unix(100*3600, 0)),
				HasStartChangepoint:          false,
				StartPositionLowerBound_99Th: 190,
				StartPositionUpperBound_99Th: 210,
				FinalizedCounts: &changepointspb.Counts{
					TotalResults:             50,
					UnexpectedResults:        3,
					TotalRuns:                40,
					UnexpectedUnretriedRuns:  5,
					UnexpectedAfterRetryRuns: 6,
					FlakyRuns:                7,
					TotalVerdicts:            20,
					UnexpectedVerdicts:       3,
					FlakyVerdicts:            2,
				},
				MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0))},
		}
		tvb.UpdateOutputBuffer(evictedSegments)
		So(tvb.FinalizedSegments, ShouldBeNil)
		So(tvb.FinalizingSegment, ShouldNotBeNil)
		expected := &changepointspb.Segment{
			State:                        changepointspb.SegmentState_FINALIZING,
			StartPosition:                100,
			StartHour:                    timestamppb.New(time.Unix(3600, 0)),
			HasStartChangepoint:          true,
			StartPositionLowerBound_99Th: 90,
			StartPositionUpperBound_99Th: 110,
			FinalizedCounts: &changepointspb.Counts{
				TotalResults:             80,
				UnexpectedResults:        8,
				TotalRuns:                60,
				UnexpectedUnretriedRuns:  7,
				UnexpectedAfterRetryRuns: 9,
				FlakyRuns:                11,
				TotalVerdicts:            30,
				UnexpectedVerdicts:       4,
				FlakyVerdicts:            4,
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
		}
		So(tvb.FinalizingSegment, ShouldResembleProto, expected)
	})

	Convey("Combine finalizing segment with finalized segment", t, func() {
		tvb := TestVariantBranch{
			FinalizingSegment: &changepointspb.Segment{
				State:                        changepointspb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
				FinalizedCounts: &changepointspb.Counts{
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
		evictedSegments := []*changepointspb.Segment{
			{
				State:                        changepointspb.SegmentState_FINALIZED,
				StartPosition:                200,
				StartHour:                    timestamppb.New(time.Unix(100*3600, 0)),
				HasStartChangepoint:          false,
				StartPositionLowerBound_99Th: 190,
				StartPositionUpperBound_99Th: 210,
				EndPosition:                  400,
				EndHour:                      timestamppb.New(time.Unix(400*3600, 0)),
				FinalizedCounts: &changepointspb.Counts{
					TotalResults:             50,
					UnexpectedResults:        3,
					TotalRuns:                40,
					UnexpectedUnretriedRuns:  5,
					UnexpectedAfterRetryRuns: 6,
					FlakyRuns:                7,
					TotalVerdicts:            20,
					UnexpectedVerdicts:       3,
					FlakyVerdicts:            2,
				},
				MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
			},
			{
				State:         changepointspb.SegmentState_FINALIZING,
				StartPosition: 500,
				EndPosition:   800,
				FinalizedCounts: &changepointspb.Counts{
					TotalResults:  20,
					TotalRuns:     20,
					TotalVerdicts: 20,
				},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)
		So(len(tvb.FinalizedSegments.Segments), ShouldEqual, 1)
		So(tvb.FinalizingSegment, ShouldNotBeNil)
		So(tvb.FinalizingSegment, ShouldResembleProto, evictedSegments[1])
		expected := &changepointspb.Segment{
			State:                        changepointspb.SegmentState_FINALIZED,
			StartPosition:                100,
			StartHour:                    timestamppb.New(time.Unix(3600, 0)),
			HasStartChangepoint:          true,
			StartPositionLowerBound_99Th: 90,
			StartPositionUpperBound_99Th: 110,
			EndPosition:                  400,
			EndHour:                      timestamppb.New(time.Unix(400*3600, 0)),
			FinalizedCounts: &changepointspb.Counts{
				TotalResults:             80,
				UnexpectedResults:        8,
				TotalRuns:                60,
				UnexpectedUnretriedRuns:  7,
				UnexpectedAfterRetryRuns: 9,
				FlakyRuns:                11,
				TotalVerdicts:            30,
				UnexpectedVerdicts:       4,
				FlakyVerdicts:            4,
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
		}
		So(tvb.FinalizedSegments.Segments[0], ShouldResembleProto, expected)
	})

	Convey("Combine finalizing segment with finalized segment, with a token of finalizing segment in input buffer", t, func() {
		tvb := TestVariantBranch{
			FinalizingSegment: &changepointspb.Segment{
				State:                        changepointspb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
				FinalizedCounts: &changepointspb.Counts{
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
		evictedSegments := []*changepointspb.Segment{
			{
				State:                        changepointspb.SegmentState_FINALIZED,
				StartPosition:                200,
				StartHour:                    timestamppb.New(time.Unix(100*3600, 0)),
				HasStartChangepoint:          false,
				StartPositionLowerBound_99Th: 190,
				StartPositionUpperBound_99Th: 210,
				EndPosition:                  400,
				EndHour:                      timestamppb.New(time.Unix(400*3600, 0)),
				FinalizedCounts: &changepointspb.Counts{
					TotalResults:             50,
					UnexpectedResults:        3,
					TotalRuns:                40,
					UnexpectedUnretriedRuns:  5,
					UnexpectedAfterRetryRuns: 6,
					FlakyRuns:                7,
					TotalVerdicts:            20,
					UnexpectedVerdicts:       3,
					FlakyVerdicts:            2,
				},
				MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
			},
			{
				State:                        changepointspb.SegmentState_FINALIZING,
				StartPosition:                500,
				StartHour:                    timestamppb.New(time.Unix(500*3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 490,
				StartPositionUpperBound_99Th: 510,
				FinalizedCounts:              &changepointspb.Counts{},
			},
		}
		tvb.UpdateOutputBuffer(evictedSegments)
		So(len(tvb.FinalizedSegments.Segments), ShouldEqual, 1)
		So(tvb.FinalizingSegment, ShouldNotBeNil)
		expected := &changepointspb.Segment{
			State:                        changepointspb.SegmentState_FINALIZED,
			StartPosition:                100,
			StartHour:                    timestamppb.New(time.Unix(3600, 0)),
			HasStartChangepoint:          true,
			StartPositionLowerBound_99Th: 90,
			StartPositionUpperBound_99Th: 110,
			EndPosition:                  400,
			EndHour:                      timestamppb.New(time.Unix(400*3600, 0)),
			FinalizedCounts: &changepointspb.Counts{
				TotalResults:             80,
				UnexpectedResults:        8,
				TotalRuns:                60,
				UnexpectedUnretriedRuns:  7,
				UnexpectedAfterRetryRuns: 9,
				FlakyRuns:                11,
				TotalVerdicts:            30,
				UnexpectedVerdicts:       4,
				FlakyVerdicts:            4,
			},
			MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
		}
		So(tvb.FinalizedSegments.Segments[0], ShouldResembleProto, expected)
		So(tvb.FinalizingSegment, ShouldResembleProto, evictedSegments[1])
	})

	Convey("Should panic if no finalizing segment in evicted segments", t, func() {
		tvb := TestVariantBranch{
			FinalizingSegment: &changepointspb.Segment{
				State:                        changepointspb.SegmentState_FINALIZING,
				StartPosition:                100,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				HasStartChangepoint:          true,
				StartPositionLowerBound_99Th: 90,
				StartPositionUpperBound_99Th: 110,
				FinalizedCounts: &changepointspb.Counts{
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
		evictedSegments := []*changepointspb.Segment{
			{
				State:                        changepointspb.SegmentState_FINALIZED,
				StartPosition:                200,
				StartHour:                    timestamppb.New(time.Unix(100*3600, 0)),
				HasStartChangepoint:          false,
				StartPositionLowerBound_99Th: 190,
				StartPositionUpperBound_99Th: 210,
				EndPosition:                  400,
				EndHour:                      timestamppb.New(time.Unix(400*3600, 0)),
				FinalizedCounts: &changepointspb.Counts{
					TotalResults:             50,
					UnexpectedResults:        3,
					TotalRuns:                40,
					UnexpectedUnretriedRuns:  5,
					UnexpectedAfterRetryRuns: 6,
					FlakyRuns:                7,
					TotalVerdicts:            20,
					UnexpectedVerdicts:       3,
					FlakyVerdicts:            2,
				},
				MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(10*3600, 0)),
			},
		}
		f := func() { tvb.UpdateOutputBuffer(evictedSegments) }
		So(f, ShouldPanic)
	})
}

func makeTestVariantBranchKey(proj string, testID string, variantHash string, refHash RefHash) TestVariantBranchKey {
	return TestVariantBranchKey{
		Project:     proj,
		TestID:      testID,
		VariantHash: variantHash,
		RefHash:     refHash,
	}
}
