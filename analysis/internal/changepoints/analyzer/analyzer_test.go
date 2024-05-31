// Copyright 2024 The LUCI Authors.
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

package analyzer

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAnalyzer(t *testing.T) {
	Convey("Analyzer", t, func() {

		var a Analyzer

		variant := &pb.Variant{
			Def: map[string]string{
				"k": "v",
			},
		}

		sourceRef := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
				},
			},
		}

		Convey("test variant branch without finalizing or finalized segments", func() {
			tvb := &testvariantbranch.Entry{
				Project:     "chromium",
				TestID:      "test_id_1",
				VariantHash: "variant_hash_1",
				RefHash:     []byte("refhash1"),
				Variant:     variant,
				SourceRef:   sourceRef,
				InputBuffer: &inputbuffer.Buffer{
					HotBufferCapacity:  10,
					HotBuffer:          inputbuffer.History{},
					ColdBufferCapacity: 100,
					ColdBuffer:         inputbuffer.History{},
				},
			}

			verdicts := []inputbuffer.PositionVerdict{
				{
					CommitPosition:       1,
					Hour:                 time.Unix(1000*3600, 0),
					IsSimpleExpectedPass: true,
				},
				{
					CommitPosition:       1,
					Hour:                 time.Unix(1001*3600, 0),
					IsSimpleExpectedPass: true,
				},
				{
					CommitPosition: 2,
					Hour:           time.Unix(2000*3600, 0),
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
				{
					CommitPosition: 2,
					Hour:           time.Unix(2001*3600, 0),
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
			}

			expectedSegments := []Segment{
				{
					StartPosition:                  1,
					StartHour:                      time.Unix(1000*3600, 0),
					EndPosition:                    2,
					EndHour:                        time.Unix(2001*3600, 0),
					MostRecentUnexpectedResultHour: time.Unix(2001*3600, 0),
					Counts: Counts{
						TotalResults:             4,
						UnexpectedResults:        2,
						ExpectedPassedResults:    2,
						UnexpectedFailedResults:  1,
						UnexpectedCrashedResults: 1,

						TotalRuns:               4,
						UnexpectedUnretriedRuns: 2,

						TotalVerdicts:      4,
						UnexpectedVerdicts: 2,
					},
				},
			}
			Convey("without eviction", func() {
				for _, v := range verdicts {
					tvb.InputBuffer.InsertVerdict(v)
				}

				segments := a.Run(tvb)
				So(segments, ShouldResembleProto, expectedSegments)
			})
			Convey("with eviction", func() {
				tvb.InputBuffer.ColdBufferCapacity = 2
				tvb.InputBuffer.HotBufferCapacity = 1

				for _, v := range verdicts {
					tvb.InputBuffer.InsertVerdict(v)
				}

				segments := a.Run(tvb)
				So(segments, ShouldResembleProto, expectedSegments)

				So(tvb.InputBuffer, ShouldResembleProto, &inputbuffer.Buffer{
					HotBufferCapacity: 1,
					HotBuffer: inputbuffer.History{
						Verdicts: []inputbuffer.PositionVerdict{},
					},
					ColdBufferCapacity: 2,
					ColdBuffer: inputbuffer.History{
						Verdicts: []inputbuffer.PositionVerdict{
							{
								CommitPosition: 2,
								Hour:           time.Unix(2000*3600, 0),
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
							{
								CommitPosition: 2,
								Hour:           time.Unix(2001*3600, 0),
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
					IsColdBufferDirty: true,
				})
				So(tvb.FinalizingSegment, ShouldResembleProto, &cpb.Segment{
					State:         cpb.SegmentState_FINALIZING,
					StartPosition: 1,
					StartHour:     timestamppb.New(time.Unix(1000*3600, 0)),

					FinalizedCounts: &cpb.Counts{
						TotalResults:          2,
						ExpectedPassedResults: 2,
						TotalRuns:             2,
						TotalVerdicts:         2,
					},
				})
				So(tvb.IsFinalizingSegmentDirty, ShouldBeTrue)
				So(tvb.FinalizedSegments, ShouldBeNil)
				So(tvb.IsFinalizedSegmentsDirty, ShouldBeFalse)
			})
		})
		Convey("analyze test variant branch with finalizing and finalized segments", func() {
			var (
				positions     = []int{30, 31, 32, 33, 34, 35, 36, 37, 38, 39}
				total         = []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}
				hasUnexpected = []int{5, 5, 5, 5, 5, 0, 0, 0, 0, 0}
			)

			row := &testvariantbranch.Entry{
				Project:     "chromium",
				TestID:      "test_id_2",
				VariantHash: "variant_hash_2",
				RefHash:     []byte("refhash2"),
				Variant:     variant,
				SourceRef:   sourceRef,
				InputBuffer: &inputbuffer.Buffer{
					HotBufferCapacity: 10,
					HotBuffer: inputbuffer.History{
						Verdicts: inputbuffer.Verdicts(positions, total, hasUnexpected),
					},
					ColdBufferCapacity: 10,
				},
				FinalizedSegments: &cpb.Segments{
					Segments: []*cpb.Segment{
						{
							State:                          cpb.SegmentState_FINALIZED,
							HasStartChangepoint:            false,
							StartPosition:                  1,
							StartHour:                      timestamppb.New(time.Unix(7000*3600, 0)),
							EndPosition:                    10,
							EndHour:                        timestamppb.New(time.Unix(8000*3600, 0)),
							MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(9000*3600, 0)),
							FinalizedCounts: &cpb.Counts{
								TotalResults:      9,
								UnexpectedResults: 8,

								TotalRuns:                7,
								UnexpectedUnretriedRuns:  6,
								UnexpectedAfterRetryRuns: 5,
								FlakyRuns:                4,

								TotalVerdicts:      3,
								UnexpectedVerdicts: 2,
								FlakyVerdicts:      1,
							},
						},
						{
							State:                          cpb.SegmentState_FINALIZED,
							HasStartChangepoint:            true,
							StartPosition:                  11,
							StartHour:                      timestamppb.New(time.Unix(7000*3600, 0)),
							StartPositionLowerBound_99Th:   9,
							StartPositionUpperBound_99Th:   13,
							EndPosition:                    20,
							EndHour:                        timestamppb.New(time.Unix(8000*3600, 0)),
							MostRecentUnexpectedResultHour: timestamppb.New(time.Time{}),
							FinalizedCounts: &cpb.Counts{
								TotalVerdicts: 5,
							},
						},
					},
				},
				FinalizingSegment: &cpb.Segment{
					State:                          cpb.SegmentState_FINALIZING,
					HasStartChangepoint:            true,
					StartPosition:                  21,
					EndPosition:                    30,
					StartHour:                      timestamppb.New(time.Unix(7000*3600, 0)),
					StartPositionLowerBound_99Th:   19,
					StartPositionUpperBound_99Th:   23,
					EndHour:                        timestamppb.New(time.Unix(8000*3600, 0)),
					MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(9000*3600, 0)),
					FinalizedCounts: &cpb.Counts{
						TotalResults:             10,
						UnexpectedResults:        9,
						UnexpectedCrashedResults: 8,

						TotalRuns:                7,
						FlakyRuns:                6,
						UnexpectedUnretriedRuns:  5,
						UnexpectedAfterRetryRuns: 4,

						TotalVerdicts:      3,
						UnexpectedVerdicts: 2,
						FlakyVerdicts:      1,
					},
				},
			}

			expectedSegments := []Segment{
				{
					HasStartChangepoint:         true,
					StartPosition:               35,
					StartPositionLowerBound99Th: 34,
					StartPositionUpperBound99Th: 35,
					StartHour:                   time.Unix(35*3600, 0),
					EndPosition:                 39,
					EndHour:                     time.Unix(39*3600, 0),
					Counts: Counts{
						TotalResults:          25,
						ExpectedPassedResults: 25,
						TotalRuns:             25,
						TotalVerdicts:         5,
					},
				},
				{
					HasStartChangepoint:            true,
					StartPosition:                  21,
					StartPositionLowerBound99Th:    19,
					StartPositionUpperBound99Th:    23,
					StartHour:                      time.Unix(7000*3600, 0),
					EndPosition:                    34,
					EndHour:                        time.Unix(34*3600, 0),
					MostRecentUnexpectedResultHour: time.Unix(9000*3600, 0),
					Counts: Counts{
						TotalResults:             10 + 25, // 10+25
						UnexpectedResults:        9 + 25,  // 9+25
						UnexpectedCrashedResults: 8,
						UnexpectedFailedResults:  25, // from input buffer

						TotalRuns:                7 + 25,
						FlakyRuns:                6,
						UnexpectedUnretriedRuns:  5 + 25,
						UnexpectedAfterRetryRuns: 4,

						TotalVerdicts:      3 + 5,
						UnexpectedVerdicts: 2 + 5,
						FlakyVerdicts:      1,
					},
				},
				{
					HasStartChangepoint:            true,
					StartPosition:                  11,
					StartPositionLowerBound99Th:    9,
					StartPositionUpperBound99Th:    13,
					StartHour:                      time.Unix(7000*3600, 0),
					EndPosition:                    20,
					EndHour:                        time.Unix(8000*3600, 0),
					MostRecentUnexpectedResultHour: time.Time{},
					Counts:                         Counts{TotalVerdicts: 5},
				},
				{
					StartPosition:                  1,
					StartHour:                      time.Unix(7000*3600, 0),
					EndPosition:                    10,
					EndHour:                        time.Unix(8000*3600, 0),
					MostRecentUnexpectedResultHour: time.Unix(9000*3600, 0),
					Counts: Counts{
						TotalResults:      9,
						UnexpectedResults: 8,

						TotalRuns:                7,
						UnexpectedUnretriedRuns:  6,
						UnexpectedAfterRetryRuns: 5,
						FlakyRuns:                4,

						TotalVerdicts:      3,
						UnexpectedVerdicts: 2,
						FlakyVerdicts:      1,
					},
				},
			}

			segments := a.Run(row)
			So(segments, ShouldResembleProto, expectedSegments)
		})
	})
}
