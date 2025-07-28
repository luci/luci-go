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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/model"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestAnalyzer(t *testing.T) {
	ftt.Run("Analyzer", t, func(t *ftt.Test) {
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

		t.Run("test variant branch without finalizing or finalized segments", func(t *ftt.Test) {
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

			runs := []inputbuffer.Run{
				{
					CommitPosition: 1,
					Hour:           time.Unix(1000*3600, 0),
					Expected: inputbuffer.ResultCounts{
						PassCount: 1,
					},
				},
				{
					CommitPosition: 1,
					Hour:           time.Unix(1001*3600, 0),
					Expected: inputbuffer.ResultCounts{
						PassCount: 1,
					},
				},
				{
					CommitPosition: 2,
					Hour:           time.Unix(2000*3600, 0),
					Unexpected: inputbuffer.ResultCounts{
						FailCount: 1,
					},
				},
				{
					CommitPosition: 2,
					Hour:           time.Unix(2001*3600, 0),
					Unexpected: inputbuffer.ResultCounts{
						CrashCount: 1,
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

						TotalSourceVerdicts:      2,
						UnexpectedSourceVerdicts: 1,
					},
				},
			}
			t.Run("without eviction", func(t *ftt.Test) {
				for _, run := range runs {
					tvb.InputBuffer.InsertRun(run)
				}

				segments := a.Run(tvb)
				assert.Loosely(t, segments, should.Match(expectedSegments))
			})
			t.Run("with eviction", func(t *ftt.Test) {
				tvb.InputBuffer.ColdBufferCapacity = 2
				tvb.InputBuffer.HotBufferCapacity = 1

				for _, run := range runs {
					tvb.InputBuffer.InsertRun(run)
				}

				segments := a.Run(tvb)
				assert.Loosely(t, segments, should.Match(expectedSegments))

				assert.Loosely(t, tvb.InputBuffer, should.Match(&inputbuffer.Buffer{
					HotBufferCapacity: 1,
					HotBuffer: inputbuffer.History{
						Runs: []inputbuffer.Run{},
					},
					ColdBufferCapacity: 2,
					ColdBuffer: inputbuffer.History{
						Runs: []inputbuffer.Run{
							{
								CommitPosition: 2,
								Hour:           time.Unix(2000*3600, 0),
								Unexpected: inputbuffer.ResultCounts{
									FailCount: 1,
								},
							},
							{
								CommitPosition: 2,
								Hour:           time.Unix(2001*3600, 0),
								Unexpected: inputbuffer.ResultCounts{
									CrashCount: 1,
								},
							},
						},
					},
					IsColdBufferDirty: true,
				}))
				assert.Loosely(t, tvb.FinalizingSegment, should.Match(&cpb.Segment{
					State:         cpb.SegmentState_FINALIZING,
					StartPosition: 1,
					StartHour:     timestamppb.New(time.Unix(1000*3600, 0)),

					FinalizedCounts: &cpb.Counts{
						TotalResults:          2,
						ExpectedPassedResults: 2,
						TotalRuns:             2,
						PartialSourceVerdict: &cpb.PartialSourceVerdict{
							CommitPosition:  1,
							LastHour:        timestamppb.New(time.Unix(1001*3600, 0)),
							ExpectedResults: 2,
						},
					},
				}))
				assert.Loosely(t, tvb.IsFinalizingSegmentDirty, should.BeTrue)
				assert.Loosely(t, tvb.FinalizedSegments, should.BeNil)
				assert.Loosely(t, tvb.IsFinalizedSegmentsDirty, should.BeFalse)
			})
		})
		t.Run("analyze test variant branch with finalizing and finalized segments", func(t *ftt.Test) {
			var (
				positions     = []int{30, 31, 32, 33, 34, 35, 36, 37, 38, 39}
				total         = []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}
				hasUnexpected = []int{5, 5, 5, 5, 0, 0, 0, 0, 0, 1}
			)

			row := &testvariantbranch.Entry{
				Project:     "chromium",
				TestID:      "test_id_2",
				VariantHash: "variant_hash_2",
				RefHash:     []byte("refhash2"),
				Variant:     variant,
				SourceRef:   sourceRef,
				InputBuffer: &inputbuffer.Buffer{
					HotBufferCapacity: 11,
					HotBuffer: inputbuffer.History{
						Runs: inputbuffer.Verdicts(positions[:2], total[:2], hasUnexpected[:2]),
					},
					ColdBufferCapacity: 50,
					ColdBuffer: inputbuffer.History{
						Runs: inputbuffer.Verdicts(positions[2:], total[2:], hasUnexpected[2:]),
					},
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

								TotalSourceVerdicts:      3,
								UnexpectedSourceVerdicts: 2,
								FlakySourceVerdicts:      1,
							},
						},
						{
							State:                          cpb.SegmentState_FINALIZED,
							HasStartChangepoint:            true,
							StartPosition:                  11,
							StartHour:                      timestamppb.New(time.Unix(7000*3600, 0)),
							StartPositionLowerBound_99Th:   9,
							StartPositionUpperBound_99Th:   13,
							StartPositionDistribution:      model.SimpleDistribution(11, 2).Serialize(),
							EndPosition:                    20,
							EndHour:                        timestamppb.New(time.Unix(8000*3600, 0)),
							MostRecentUnexpectedResultHour: timestamppb.New(time.Time{}),
							FinalizedCounts: &cpb.Counts{
								TotalSourceVerdicts: 5,
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
					StartPositionDistribution:      model.SimpleDistribution(21, 2).Serialize(),
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

						TotalSourceVerdicts:      3,
						UnexpectedSourceVerdicts: 2,
						FlakySourceVerdicts:      1,
						PartialSourceVerdict: &cpb.PartialSourceVerdict{
							CommitPosition:  30,
							LastHour:        timestamppb.New(time.Unix(7900*3600, 0)),
							ExpectedResults: 1,
						},
					},
				},
			}

			expectedSegments := []Segment{
				{
					HasStartChangepoint:            true,
					StartPosition:                  34,
					StartPositionLowerBound99Th:    34,
					StartPositionUpperBound99Th:    34,
					StartPositionDistribution:      model.SimpleDistribution(34, 0),
					StartHour:                      time.Unix(34*3600, 0),
					EndPosition:                    39,
					EndHour:                        time.Unix(39*3600, 0),
					MostRecentUnexpectedResultHour: time.Unix(39*3600, 0),
					Counts: Counts{
						TotalResults:            30,
						UnexpectedResults:       1,
						ExpectedPassedResults:   29,
						UnexpectedFailedResults: 1,
						TotalRuns:               30,
						UnexpectedUnretriedRuns: 1,
						TotalSourceVerdicts:     6,
						FlakySourceVerdicts:     1,
					},
				},
				{
					HasStartChangepoint:            true,
					StartPosition:                  21,
					StartPositionLowerBound99Th:    19,
					StartPositionUpperBound99Th:    23,
					StartPositionDistribution:      model.SimpleDistribution(21, 2),
					StartHour:                      time.Unix(7000*3600, 0),
					EndPosition:                    33,
					EndHour:                        time.Unix(33*3600, 0),
					MostRecentUnexpectedResultHour: time.Unix(9000*3600, 0),
					Counts: Counts{
						TotalResults:             10 + 20, // 10+25
						UnexpectedResults:        9 + 20,  // 9+25
						UnexpectedCrashedResults: 8,
						UnexpectedFailedResults:  20, // from input buffer

						TotalRuns:                7 + 20,
						FlakyRuns:                6,
						UnexpectedUnretriedRuns:  5 + 20,
						UnexpectedAfterRetryRuns: 4,

						TotalSourceVerdicts:      3 + 4,
						UnexpectedSourceVerdicts: 2 + 3,
						FlakySourceVerdicts:      1 + 1, // From merging with expected partial source verdict in finalizing segment.
					},
				},
				{
					HasStartChangepoint:            true,
					StartPosition:                  11,
					StartPositionLowerBound99Th:    9,
					StartPositionUpperBound99Th:    13,
					StartPositionDistribution:      model.SimpleDistribution(11, 2),
					StartHour:                      time.Unix(7000*3600, 0),
					EndPosition:                    20,
					EndHour:                        time.Unix(8000*3600, 0),
					MostRecentUnexpectedResultHour: time.Time{},
					Counts:                         Counts{TotalSourceVerdicts: 5},
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

						TotalSourceVerdicts:      3,
						UnexpectedSourceVerdicts: 2,
						FlakySourceVerdicts:      1,
					},
				},
			}

			t.Run("without eviction", func(t *ftt.Test) {
				// By increasing capacity, we avoid eviction due to
				// the changepoint occuring in the old half of the input
				// buffer.
				row.InputBuffer.ColdBufferCapacity = 100

				segments := a.Run(row)
				assert.Loosely(t, segments, should.Match(expectedSegments))
				assert.Loosely(t, row.InputBuffer.IsColdBufferDirty, should.BeFalse)
			})
			t.Run("with eviction forced by space pressure", func(t *ftt.Test) {
				row.InputBuffer.ColdBufferCapacity = 10
				row.InputBuffer.HotBufferCapacity = 5

				segments := a.Run(row)
				assert.Loosely(t, segments, should.Match(expectedSegments))
				assert.Loosely(t, row.InputBuffer.IsColdBufferDirty, should.BeTrue)

				assert.Loosely(t, row.InputBuffer.HotBuffer.Runs, should.HaveLength(0))
				assert.Loosely(t, row.InputBuffer.ColdBuffer.Runs, should.HaveLength(10))
				assert.Loosely(t, row.FinalizedSegments.Segments, should.HaveLength(3))
				assert.Loosely(t, row.FinalizedSegments.Segments[2].StartPosition, should.Equal(21))
				assert.Loosely(t, row.FinalizedSegments.Segments[2].EndPosition, should.Equal(33))
				assert.Loosely(t, row.FinalizingSegment.StartPosition, should.Equal(34))
				assert.Loosely(t, row.FinalizingSegment.FinalizedCounts, should.Match(&cpb.Counts{
					TotalResults:          20,
					ExpectedPassedResults: 20,
					TotalRuns:             20,
					TotalSourceVerdicts:   3,
					PartialSourceVerdict: &cpb.PartialSourceVerdict{
						CommitPosition:  37,
						LastHour:        timestamppb.New(time.Unix(37*3600, 0)),
						ExpectedResults: 5,
					},
				}))
			})
			t.Run("with eviction due to changepoint", func(t *ftt.Test) {
				segments := a.Run(row)
				assert.Loosely(t, segments, should.Match(expectedSegments))
				assert.Loosely(t, row.InputBuffer.IsColdBufferDirty, should.BeTrue)

				assert.Loosely(t, row.InputBuffer.HotBuffer.Runs, should.HaveLength(0))
				assert.Loosely(t, row.InputBuffer.ColdBuffer.Runs, should.HaveLength(30))
				assert.Loosely(t, row.FinalizedSegments.Segments, should.HaveLength(3))
				assert.Loosely(t, row.FinalizedSegments.Segments[2].StartPosition, should.Equal(21))
				assert.Loosely(t, row.FinalizedSegments.Segments[2].EndPosition, should.Equal(33))
				assert.Loosely(t, row.FinalizingSegment.StartPosition, should.Equal(34))
				assert.Loosely(t, row.FinalizingSegment.FinalizedCounts, should.Match(&cpb.Counts{}))
			})
		})
		t.Run("legacy test variant branch which has more runs than capacity", func(t *ftt.Test) {
			// Legacy data formats may be storing more runs than is now the buffer capacity.
			// E.g. version 2 was limited to 2000 verdicts (not runs).

			var hotRuns []inputbuffer.Run
			for i := range 300 {
				hotRuns = append(hotRuns, inputbuffer.Run{
					CommitPosition: int64(i + 1000),
					Hour:           time.Unix(int64(i)*3600, 0),
					Expected:       inputbuffer.ResultCounts{PassCount: 1},
				})
			}
			var coldRuns []inputbuffer.Run
			for i := range 6000 {
				coldRuns = append(coldRuns, inputbuffer.Run{
					CommitPosition: int64(i + 1300),
					Hour:           time.Unix(int64(i+1300)*3600, 0),
					Expected:       inputbuffer.ResultCounts{PassCount: 1},
				})
			}

			row := &testvariantbranch.Entry{
				Project:     "chromium",
				TestID:      "test_id_2",
				VariantHash: "variant_hash_2",
				RefHash:     []byte("refhash2"),
				Variant:     variant,
				SourceRef:   sourceRef,
				InputBuffer: &inputbuffer.Buffer{
					HotBufferCapacity: 1,
					HotBuffer: inputbuffer.History{
						Runs: hotRuns,
					},
					ColdBufferCapacity: 2,
					ColdBuffer: inputbuffer.History{
						Runs: coldRuns,
					},
				},
				FinalizingSegment: &cpb.Segment{
					State:               cpb.SegmentState_FINALIZING,
					HasStartChangepoint: true,
					StartPosition:       1,
					StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
					EndPosition:         999,
					FinalizedCounts: &cpb.Counts{
						TotalResults:          999,
						ExpectedPassedResults: 999,
						TotalRuns:             999,
						TotalSourceVerdicts:   999,
					},
				},
			}

			expectedSegments := []Segment{
				{
					HasStartChangepoint: true,
					StartPosition:       1,
					StartHour:           time.Unix(1*3600, 0),
					EndPosition:         7299,
					EndHour:             time.Unix(7299*3600, 0),
					Counts: Counts{
						TotalResults:          7299,
						ExpectedPassedResults: 7299,
						TotalRuns:             7299,
						TotalSourceVerdicts:   7299,
					},
				},
			}
			segments := a.Run(row)
			assert.Loosely(t, segments, should.Match(expectedSegments))
		})
	})
}

// Output as of June 2024 on Intel Skylake CPU @ 2.00GHz:
// BenchmarkAnalyzer-96               20276             58743 ns/op             928 B/op         11 allocs/op
func BenchmarkAnalyzer(b *testing.B) {
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

	// Tests analysis on a consistently passing test, which is most
	// test variants.
	var hotRuns []inputbuffer.Run
	for i := range 100 {
		hotRuns = append(hotRuns, inputbuffer.Run{
			CommitPosition: int64(i + 1000),
			Hour:           time.Unix(int64(i)*3600, 0),
			Expected:       inputbuffer.ResultCounts{PassCount: 1},
		})
	}
	var coldRuns []inputbuffer.Run
	for i := range 2000 {
		coldRuns = append(coldRuns, inputbuffer.Run{
			CommitPosition: int64(i + 1300),
			Hour:           time.Unix(int64(i+1300)*3600, 0),
			Expected:       inputbuffer.ResultCounts{PassCount: 1},
		})
	}

	var a Analyzer
	for i := 0; i < b.N; i++ {
		row := &testvariantbranch.Entry{
			Project:     "chromium",
			TestID:      "test_id_2",
			VariantHash: "variant_hash_2",
			RefHash:     []byte("refhash2"),
			Variant:     variant,
			SourceRef:   sourceRef,
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity: 101,
				HotBuffer: inputbuffer.History{
					Runs: hotRuns,
				},
				ColdBufferCapacity: 2000,
				ColdBuffer: inputbuffer.History{
					Runs: coldRuns,
				},
			},
			FinalizingSegment: &cpb.Segment{
				State:               cpb.SegmentState_FINALIZING,
				HasStartChangepoint: true,
				StartPosition:       1,
				StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
				EndPosition:         999,
				FinalizedCounts: &cpb.Counts{
					TotalResults:          999,
					ExpectedPassedResults: 999,
					TotalRuns:             999,
					TotalSourceVerdicts:   999,
				},
			},
		}
		segments := a.Run(row)
		if len(segments) != 1 {
			panic("wrong number of segments")
		}
		if len(row.InputBuffer.HotBuffer.Runs) != 100 {
			panic("unexpected hot buffer eviction")
		}
		if len(row.InputBuffer.ColdBuffer.Runs) != 2000 {
			panic("unexpected cold buffer eviction")
		}
	}
}

// Output as of June 2024 on Intel Skylake CPU @ 2.00GHz:
// BenchmarkAnalyzerWithChangepoint-96    	    1118	   1019629 ns/op	  122230 B/op	      43 allocs/op
//
// Analysis indicates that most of the time is spent identifying the changepoint confidence interval.
func BenchmarkAnalyzerWithChangepoint(b *testing.B) {
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

	var a Analyzer
	var hotRuns []inputbuffer.Run
	var coldRuns []inputbuffer.Run
	for i := range 100 {
		hotRuns = append(hotRuns, inputbuffer.Run{
			CommitPosition: int64(i + 3000),
			Hour:           time.Unix(int64(i)*3600, 0),
			Unexpected:     inputbuffer.ResultCounts{FailCount: 1},
		})
	}

	for i := range 2000 {
		coldRuns = append(coldRuns, inputbuffer.Run{
			CommitPosition: int64(i + 1000),
			Hour:           time.Unix(int64(i+1300)*3600, 0),
			Expected:       inputbuffer.ResultCounts{PassCount: 1},
		})
	}

	for i := 0; i < b.N; i++ {
		row := &testvariantbranch.Entry{
			Project:     "chromium",
			TestID:      "test_id_2",
			VariantHash: "variant_hash_2",
			RefHash:     []byte("refhash2"),
			Variant:     variant,
			SourceRef:   sourceRef,
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity: 101,
				HotBuffer: inputbuffer.History{
					Runs: hotRuns,
				},
				ColdBufferCapacity: 2000,
				ColdBuffer: inputbuffer.History{
					Runs: coldRuns,
				},
			},
			FinalizingSegment: &cpb.Segment{
				State:               cpb.SegmentState_FINALIZING,
				HasStartChangepoint: true,
				StartPosition:       1,
				StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
				EndPosition:         999,
				FinalizedCounts: &cpb.Counts{
					TotalResults:          999,
					ExpectedPassedResults: 999,
					TotalRuns:             999,
					TotalSourceVerdicts:   999,
				},
			},
		}
		segments := a.Run(row)
		if len(segments) != 2 {
			panic("wrong number of segments")
		}
		if len(row.InputBuffer.HotBuffer.Runs) != 100 {
			panic("unexpected hot buffer eviction")
		}
		if len(row.InputBuffer.ColdBuffer.Runs) != 2000 {
			panic("unexpected cold buffer eviction")
		}
	}
}
