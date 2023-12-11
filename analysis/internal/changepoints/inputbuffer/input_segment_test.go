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

package inputbuffer

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSegmentizeInputBuffer(t *testing.T) {
	Convey("Segmentize input buffer", t, func() {
		Convey("No change point", func() {
			var (
				positions     = []int{1, 2, 3, 4, 5, 6}
				total         = []int{1, 2, 1, 2, 1, 2}
				hasUnexpected = []int{0, 1, 0, 2, 0, 0}
			)
			ib := genInputBuffer(10, 200, Verdicts(positions, total, hasUnexpected))
			cps := []ChangePoint{}

			var merged []PositionVerdict
			ib.MergeBuffer(&merged)
			sib := ib.Segmentize(merged, cps)
			ibSegments := sib.Segments
			So(len(ibSegments), ShouldEqual, 1)
			So(ibSegments[0], ShouldResembleProto, &Segment{
				StartIndex:          0,
				EndIndex:            5,
				HasStartChangepoint: false,
				StartPosition:       1,
				EndPosition:         6,
				StartHour:           timestamppb.New(time.Unix(3600, 0)),
				EndHour:             timestamppb.New(time.Unix(6*3600, 0)),
				Counts: &cpb.Counts{
					TotalResults:            9,
					UnexpectedResults:       3,
					TotalRuns:               9,
					UnexpectedUnretriedRuns: 3,
					TotalVerdicts:           6,
					FlakyVerdicts:           1,
					UnexpectedVerdicts:      1,
					ExpectedPassedResults:   6,
					UnexpectedFailedResults: 3,
				},
				MostRecentUnexpectedResultHourAllVerdicts: timestamppb.New(time.Unix(4*3600, 0)),
			})
		})

		Convey("With change points and retries", func() {
			var (
				positions            = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
				total                = []int{1, 1, 1, 2, 2, 2, 2, 2, 2, 1, 1, 1}
				hasUnexpected        = []int{0, 0, 0, 2, 2, 2, 2, 2, 2, 1, 1, 1}
				retries              = []int{0, 0, 0, 2, 2, 2, 2, 2, 2, 0, 0, 0}
				unexpectedAfterRetry = []int{0, 0, 0, 2, 2, 2, 0, 0, 0, 0, 0, 0}
			)
			ib := genInputbufferWithRetries(10, 200, positions, total, hasUnexpected, retries, unexpectedAfterRetry)
			cps := []ChangePoint{
				{
					NominalIndex:        3,
					LowerBound99ThIndex: 2,
					UpperBound99ThIndex: 4,
				},
				{
					NominalIndex:        6,
					LowerBound99ThIndex: 5,
					UpperBound99ThIndex: 7,
				},
				{
					NominalIndex:        9,
					LowerBound99ThIndex: 8,
					UpperBound99ThIndex: 10,
				},
			}
			var merged []PositionVerdict
			ib.MergeBuffer(&merged)
			sib := ib.Segmentize(merged, cps)
			ibSegments := sib.Segments
			So(len(ibSegments), ShouldEqual, 4)
			So(ibSegments[0], ShouldResembleProto, &Segment{
				StartIndex:          0,
				EndIndex:            2,
				HasStartChangepoint: false,
				StartPosition:       1,
				EndPosition:         3,
				StartHour:           timestamppb.New(time.Unix(3600, 0)),
				EndHour:             timestamppb.New(time.Unix(3*3600, 0)),
				Counts: &cpb.Counts{
					TotalResults:          3,
					TotalRuns:             3,
					TotalVerdicts:         3,
					ExpectedPassedResults: 3,
				},
			})

			So(ibSegments[1], ShouldResembleProto, &Segment{
				StartIndex:                  3,
				EndIndex:                    5,
				HasStartChangepoint:         true,
				StartPosition:               4,
				StartPositionLowerBound99Th: 3,
				StartPositionUpperBound99Th: 5,
				EndPosition:                 6,
				StartHour:                   timestamppb.New(time.Unix(4*3600, 0)),
				EndHour:                     timestamppb.New(time.Unix(6*3600, 0)),
				Counts: &cpb.Counts{
					TotalResults:             12,
					UnexpectedResults:        12,
					TotalRuns:                6,
					UnexpectedAfterRetryRuns: 6,
					TotalVerdicts:            3,
					UnexpectedVerdicts:       3,
					UnexpectedFailedResults:  6,
					UnexpectedCrashedResults: 6,
				},
				MostRecentUnexpectedResultHourAllVerdicts: timestamppb.New(time.Unix(6*3600, 0)),
			})

			So(ibSegments[2], ShouldResembleProto, &Segment{
				StartIndex:                  6,
				EndIndex:                    8,
				HasStartChangepoint:         true,
				StartPosition:               7,
				StartPositionLowerBound99Th: 6,
				StartPositionUpperBound99Th: 8,
				EndPosition:                 9,
				StartHour:                   timestamppb.New(time.Unix(7*3600, 0)),
				EndHour:                     timestamppb.New(time.Unix(9*3600, 0)),
				Counts: &cpb.Counts{
					TotalResults:            12,
					UnexpectedResults:       6,
					TotalRuns:               6,
					FlakyRuns:               6,
					TotalVerdicts:           3,
					FlakyVerdicts:           3,
					ExpectedPassedResults:   6,
					UnexpectedFailedResults: 6,
				},
				MostRecentUnexpectedResultHourAllVerdicts: timestamppb.New(time.Unix(9*3600, 0)),
			})

			So(ibSegments[3], ShouldResembleProto, &Segment{
				StartIndex:                  9,
				EndIndex:                    11,
				HasStartChangepoint:         true,
				StartPosition:               10,
				StartPositionLowerBound99Th: 9,
				StartPositionUpperBound99Th: 11,
				EndPosition:                 12,
				StartHour:                   timestamppb.New(time.Unix(10*3600, 0)),
				EndHour:                     timestamppb.New(time.Unix(12*3600, 0)),
				Counts: &cpb.Counts{
					TotalResults:            3,
					UnexpectedResults:       3,
					TotalRuns:               3,
					UnexpectedUnretriedRuns: 3,
					TotalVerdicts:           3,
					UnexpectedVerdicts:      3,
					UnexpectedFailedResults: 3,
				},
				MostRecentUnexpectedResultHourAllVerdicts: timestamppb.New(time.Unix(12*3600, 0)),
			})
		})
	})
}

func TestEvictSegments(t *testing.T) {
	Convey("Not evict segment", t, func() {
		ib := genInputBuffer(100, 2000, simpleVerdicts(100, 1, []int{}))
		segments := []*Segment{
			{
				StartIndex: 0,
				EndIndex:   99,
				Counts: &cpb.Counts{
					TotalResults:  100,
					TotalRuns:     100,
					TotalVerdicts: 100,
				},
				StartPosition: 1,
				EndPosition:   100,
			},
		}
		sib := &SegmentedInputBuffer{
			InputBuffer: ib,
			Segments:    segments,
		}
		evicted := sib.EvictSegments()
		remaining := sib.Segments
		So(len(evicted), ShouldEqual, 0)
		So(len(remaining), ShouldEqual, 1)
		So(ib.IsColdBufferDirty, ShouldBeFalse)
		So(remaining[0], ShouldResembleProto, segments[0])
	})

	Convey("Evict finalizing segment", t, func() {
		ib := genInputBuffer(100, 2000, simpleVerdicts(2100, 1, []int{50, 1900}))
		segments := []*Segment{
			{
				StartIndex: 0,
				EndIndex:   2049,
				Counts: &cpb.Counts{
					TotalResults:          2050,
					TotalRuns:             2050,
					TotalVerdicts:         2050,
					ExpectedPassedResults: 2050,
				},
				HasStartChangepoint: false,
				StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
				StartPosition:       1,
				EndHour:             timestamppb.New(time.Unix(2050*3600, 0)),
				EndPosition:         2050,
			},
			{
				StartIndex: 2050,
				EndIndex:   2099,
				Counts: &cpb.Counts{
					TotalResults:          50,
					TotalRuns:             50,
					TotalVerdicts:         50,
					ExpectedPassedResults: 50,
				},
				HasStartChangepoint: true,
				StartHour:           timestamppb.New(time.Unix(2051*3600, 0)),
				StartPosition:       2051,
				EndHour:             timestamppb.New(time.Unix(2100*3600, 0)),
				EndPosition:         2100,
			},
		}
		sib := &SegmentedInputBuffer{
			InputBuffer: ib,
			Segments:    segments,
		}

		evicted := sib.EvictSegments()
		remaining := sib.Segments
		So(len(evicted), ShouldEqual, 1)
		So(len(remaining), ShouldEqual, 2)
		So(ib.IsColdBufferDirty, ShouldBeTrue)

		So(evicted[0], ShouldResembleProto, EvictedSegment{
			Segment: &cpb.Segment{
				State:               cpb.SegmentState_FINALIZING,
				HasStartChangepoint: false,
				StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
				StartPosition:       1,
				FinalizedCounts: &cpb.Counts{
					TotalResults:            100,
					UnexpectedResults:       1,
					TotalRuns:               100,
					UnexpectedUnretriedRuns: 1,
					TotalVerdicts:           100,
					UnexpectedVerdicts:      1,
					ExpectedPassedResults:   99,
					UnexpectedFailedResults: 1,
				},
				MostRecentUnexpectedResultHour: timestamppb.New(time.Unix(51*3600, 0)),
			},
			Verdicts: simpleVerdicts(100, 1, []int{50}),
		})

		So(remaining[0], ShouldResembleProto, &Segment{
			StartIndex:  0,
			EndIndex:    1949,
			EndPosition: 2050,
			EndHour:     timestamppb.New(time.Unix(2050*3600, 0)),
			Counts: &cpb.Counts{
				TotalResults:            1950,
				UnexpectedResults:       1,
				TotalRuns:               1950,
				UnexpectedUnretriedRuns: 1,
				TotalVerdicts:           1950,
				UnexpectedVerdicts:      1,
				ExpectedPassedResults:   1949,
				UnexpectedFailedResults: 1,
			},
			MostRecentUnexpectedResultHourAllVerdicts: timestamppb.New(time.Unix(1901*3600, 0)),
		})

		So(remaining[1], ShouldResembleProto, &Segment{
			StartIndex:          1950,
			EndIndex:            1999,
			HasStartChangepoint: true,
			StartPosition:       2051,
			StartHour:           timestamppb.New(time.Unix(2051*3600, 0)),
			EndPosition:         2100,
			EndHour:             timestamppb.New(time.Unix(2100*3600, 0)),
			Counts: &cpb.Counts{
				TotalResults:          50,
				TotalRuns:             50,
				TotalVerdicts:         50,
				ExpectedPassedResults: 50,
			},
		})
	})

	Convey("Evict finalized segment", t, func() {
		ib := genInputBuffer(100, 2000, simpleVerdicts(2100, 1, []int{}))
		segments := []*Segment{
			{
				StartIndex: 0, // Finalized segment.
				EndIndex:   39,
				Counts: &cpb.Counts{
					TotalResults:          40,
					TotalRuns:             40,
					TotalVerdicts:         40,
					ExpectedPassedResults: 40,
				},
				HasStartChangepoint: false,
				StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
				StartPosition:       1,
				EndHour:             timestamppb.New(time.Unix(40*3600, 0)),
				EndPosition:         40,
			},
			{
				StartIndex: 40, // Finalized segment.
				EndIndex:   79,
				Counts: &cpb.Counts{
					TotalResults:          40,
					TotalRuns:             40,
					TotalVerdicts:         40,
					ExpectedPassedResults: 40,
				},
				HasStartChangepoint:         true,
				StartHour:                   timestamppb.New(time.Unix(41*3600, 0)),
				StartPositionLowerBound99Th: 30,
				StartPositionUpperBound99Th: 50,
				StartPosition:               41,
				EndHour:                     timestamppb.New(time.Unix(80*3600, 0)),
				EndPosition:                 80,
			},
			{
				StartIndex: 80, // A finalizing segment.
				EndIndex:   2049,
				Counts: &cpb.Counts{
					TotalResults:          1970,
					TotalRuns:             1970,
					TotalVerdicts:         1970,
					ExpectedPassedResults: 1970,
				},
				HasStartChangepoint:         true,
				StartHour:                   timestamppb.New(time.Unix(81*3600, 0)),
				StartPosition:               81,
				StartPositionLowerBound99Th: 70,
				StartPositionUpperBound99Th: 90,
				EndHour:                     timestamppb.New(time.Unix(2050*3600, 0)),
				EndPosition:                 2050,
			},
			{
				StartIndex: 2050, // An active segment.
				EndIndex:   2099,
				Counts: &cpb.Counts{
					TotalResults:          50,
					TotalRuns:             50,
					TotalVerdicts:         50,
					ExpectedPassedResults: 50,
				},
				HasStartChangepoint: true,
				StartHour:           timestamppb.New(time.Unix(2051*3600, 0)),
				StartPosition:       2051,
				EndHour:             timestamppb.New(time.Unix(2100*3600, 0)),
				EndPosition:         2100,
			},
		}

		sib := &SegmentedInputBuffer{
			InputBuffer: ib,
			Segments:    segments,
		}
		evicted := sib.EvictSegments()
		remaining := sib.Segments
		So(len(evicted), ShouldEqual, 3)
		So(len(remaining), ShouldEqual, 2)
		So(ib.IsColdBufferDirty, ShouldBeTrue)

		So(evicted[0], ShouldResembleProto, EvictedSegment{
			Segment: &cpb.Segment{
				State:               cpb.SegmentState_FINALIZED,
				HasStartChangepoint: false,
				StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
				StartPosition:       1,
				EndHour:             timestamppb.New(time.Unix(40*3600, 0)),
				EndPosition:         40,
				FinalizedCounts: &cpb.Counts{
					TotalResults:          40,
					TotalRuns:             40,
					TotalVerdicts:         40,
					ExpectedPassedResults: 40,
				},
			},
			Verdicts: simpleVerdicts(40, 1, []int{}),
		})

		So(evicted[1], ShouldResembleProto, EvictedSegment{
			Segment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZED,
				HasStartChangepoint:          true,
				StartHour:                    timestamppb.New(time.Unix(41*3600, 0)),
				StartPosition:                41,
				StartPositionLowerBound_99Th: 30,
				StartPositionUpperBound_99Th: 50,
				EndHour:                      timestamppb.New(time.Unix(80*3600, 0)),
				EndPosition:                  80,
				FinalizedCounts: &cpb.Counts{
					TotalResults:          40,
					TotalRuns:             40,
					TotalVerdicts:         40,
					ExpectedPassedResults: 40,
				},
			},
			Verdicts: simpleVerdicts(40, 41, []int{}),
		})

		So(evicted[2], ShouldResembleProto, EvictedSegment{
			Segment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				HasStartChangepoint:          true,
				StartHour:                    timestamppb.New(time.Unix(81*3600, 0)),
				StartPosition:                81,
				StartPositionLowerBound_99Th: 70,
				StartPositionUpperBound_99Th: 90,
				FinalizedCounts: &cpb.Counts{
					TotalResults:          20,
					TotalRuns:             20,
					TotalVerdicts:         20,
					ExpectedPassedResults: 20,
				},
			},
			Verdicts: simpleVerdicts(20, 81, []int{}),
		})

		So(remaining[0], ShouldResembleProto, &Segment{
			StartIndex:  0,
			EndIndex:    1949,
			EndPosition: 2050,
			EndHour:     timestamppb.New(time.Unix(2050*3600, 0)),
			Counts: &cpb.Counts{
				TotalResults:          1950,
				TotalRuns:             1950,
				TotalVerdicts:         1950,
				ExpectedPassedResults: 1950,
			},
		})

		So(remaining[1], ShouldResembleProto, &Segment{
			StartIndex:          1950,
			EndIndex:            1999,
			HasStartChangepoint: true,
			StartPosition:       2051,
			StartHour:           timestamppb.New(time.Unix(2051*3600, 0)),
			EndPosition:         2100,
			EndHour:             timestamppb.New(time.Unix(2100*3600, 0)),
			Counts: &cpb.Counts{
				TotalResults:          50,
				TotalRuns:             50,
				TotalVerdicts:         50,
				ExpectedPassedResults: 50,
			},
		})
	})

	Convey("Evict all hot buffer", t, func() {
		ib := genInputBuffer(100, 2000, simpleVerdicts(2000, 1, []int{}))
		ib.HotBuffer = History{
			Verdicts: []PositionVerdict{
				{
					CommitPosition: 10,
				},
			},
		}
		segments := []*Segment{
			{
				StartIndex: 0, // Finalized segment.
				EndIndex:   39,
				Counts: &cpb.Counts{
					TotalResults:          40,
					TotalRuns:             40,
					TotalVerdicts:         40,
					ExpectedPassedResults: 40,
				},
				HasStartChangepoint: false,
				StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
				StartPosition:       1,
				EndHour:             timestamppb.New(time.Unix(39*3600, 0)),
				EndPosition:         39,
			},
			{
				StartIndex: 40, // A finalizing segment.
				EndIndex:   2000,
				Counts: &cpb.Counts{
					TotalResults:          1961,
					TotalRuns:             1961,
					TotalVerdicts:         1961,
					ExpectedPassedResults: 1961,
				},
				HasStartChangepoint:         true,
				StartHour:                   timestamppb.New(time.Unix(40*3600, 0)),
				StartPosition:               40,
				StartPositionLowerBound99Th: 30,
				StartPositionUpperBound99Th: 50,
				EndHour:                     timestamppb.New(time.Unix(2000*3600, 0)),
				EndPosition:                 2000,
			},
		}

		sib := &SegmentedInputBuffer{
			InputBuffer: ib,
			Segments:    segments,
		}
		evicted := sib.EvictSegments()
		remaining := sib.Segments
		So(len(evicted), ShouldEqual, 2)
		So(len(remaining), ShouldEqual, 1)
		So(sib.InputBuffer.IsColdBufferDirty, ShouldBeTrue)

		// Hot bufffer should be empty.
		So(len(sib.InputBuffer.HotBuffer.Verdicts), ShouldEqual, 0)
		So(len(sib.InputBuffer.ColdBuffer.Verdicts), ShouldEqual, 1961)

		expectedVerdicts := []PositionVerdict{
			// The verdict in the hot buffer.
			{CommitPosition: 10},
		}
		// Plus the evicted verdicts in the cold buffer.
		expectedVerdicts = append(expectedVerdicts, simpleVerdicts(39, 1, []int{})...)

		So(evicted[0], ShouldResembleProto, EvictedSegment{
			Segment: &cpb.Segment{
				State:               cpb.SegmentState_FINALIZED,
				HasStartChangepoint: false,
				StartHour:           timestamppb.New(time.Unix(1*3600, 0)),
				StartPosition:       1,
				EndHour:             timestamppb.New(time.Unix(39*3600, 0)),
				EndPosition:         39,
				FinalizedCounts: &cpb.Counts{
					TotalResults:          40,
					TotalRuns:             40,
					TotalVerdicts:         40,
					ExpectedPassedResults: 40,
				},
			},
			Verdicts: expectedVerdicts,
		})

		So(evicted[1], ShouldResembleProto, EvictedSegment{
			Segment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				HasStartChangepoint:          true,
				StartHour:                    timestamppb.New(time.Unix(40*3600, 0)),
				StartPosition:                40,
				StartPositionLowerBound_99Th: 30,
				StartPositionUpperBound_99Th: 50,
				FinalizedCounts:              &cpb.Counts{},
			},
			Verdicts: []PositionVerdict{},
		})

		So(remaining[0], ShouldResembleProto, segments[1])
	})
}

func simpleVerdicts(verdictCount int, startPos int, unexpectedIndices []int) []PositionVerdict {
	positions := make([]int, verdictCount)
	total := make([]int, verdictCount)
	hasUnexpected := make([]int, verdictCount)
	for i := 0; i < verdictCount; i++ {
		positions[i] = i + startPos
		total[i] = 1
	}
	for _, ui := range unexpectedIndices {
		hasUnexpected[ui] = 1
	}
	return Verdicts(positions, total, hasUnexpected)
}

func genInputBuffer(hotCap int, coldCap int, history []PositionVerdict) *Buffer {
	return &Buffer{
		HotBufferCapacity:  hotCap,
		ColdBufferCapacity: coldCap,
		HotBuffer:          History{},
		ColdBuffer: History{
			Verdicts: history,
		},
	}
}

func genInputbufferWithRetries(hotCap int, coldCap int, positions, total, hasUnexpected, retried, unexpectedAfterRetry []int) *Buffer {
	history := VerdictsWithRetries(positions, total, hasUnexpected, retried, unexpectedAfterRetry)

	return &Buffer{
		HotBufferCapacity:  hotCap,
		ColdBufferCapacity: coldCap,
		HotBuffer:          History{},
		ColdBuffer: History{
			Verdicts: history,
		},
	}
}

func BenchmarkEncode(b *testing.B) {
	// Last known result (23-Jun-2023):
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkEncode-96    	   41640	     31182 ns/op	    2160 B/op	       2 allocs/op

	b.StopTimer()
	hs := &HistorySerializer{}
	hs.ensureAndClearBuf()
	ib := &Buffer{
		HotBufferCapacity:  100,
		HotBuffer:          History{Verdicts: simpleVerdicts(100, 1, []int{5})},
		ColdBufferCapacity: 2000,
		ColdBuffer:         History{Verdicts: simpleVerdicts(2000, 1, []int{102, 174, 872, 971})},
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		hs.Encode(ib.ColdBuffer)
		hs.Encode(ib.HotBuffer)
	}
}

func BenchmarkDecode(b *testing.B) {
	// Last known result (23-Jun-2023):
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkDecode-96    	   20103	     59558 ns/op	     216 B/op	       7 allocs/op

	b.StopTimer()
	var hs HistorySerializer
	hs.ensureAndClearBuf()
	inputBuffer := NewWithCapacity(100, 2000)

	ib := &Buffer{
		HotBufferCapacity:  100,
		HotBuffer:          History{Verdicts: simpleVerdicts(100, 1, []int{5})},
		ColdBufferCapacity: 2000,
		ColdBuffer:         History{Verdicts: simpleVerdicts(2000, 1, []int{102, 174, 872, 971})},
	}
	encodedColdBuffer := hs.Encode(ib.ColdBuffer) // 62 bytes compressed
	encodedHotBuffer := hs.Encode(ib.HotBuffer)   // 38 bytes compressed
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		err := hs.DecodeInto(&inputBuffer.ColdBuffer, encodedColdBuffer)
		if err != nil {
			panic(err)
		}
		err = hs.DecodeInto(&inputBuffer.HotBuffer, encodedHotBuffer)
		if err != nil {
			panic(err)
		}
	}
}
