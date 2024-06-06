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
	"sort"
	"testing"
	"time"

	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSegmentizeInputBuffer(t *testing.T) {
	Convey("Segmentize input buffer", t, func() {
		Convey("No change point", func() {
			var (
				// index      = []int{0, 1, 3, 4, 6, 7}
				positions     = []int{1, 2, 3, 4, 5, 6}
				total         = []int{1, 2, 1, 2, 1, 2}
				hasUnexpected = []int{0, 1, 0, 2, 0, 0}
			)
			ib := genInputBuffer(10, 200, Verdicts(positions, total, hasUnexpected))
			cps := []ChangePoint{}

			var merged []*Run
			ib.MergeBuffer(&merged)
			sib := ib.Segmentize(merged, cps)
			ibSegments := sib.Segments
			So(len(ibSegments), ShouldEqual, 1)
			So(ibSegments[0], ShouldResembleProto, &Segment{
				StartIndex:                     0,
				EndIndex:                       8, // runs
				HasStartChangepoint:            false,
				StartPosition:                  1,
				EndPosition:                    6,
				StartHour:                      time.Unix(3600, 0),
				EndHour:                        time.Unix(6*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(4*3600, 0),
			})
		})

		Convey("With change points and retries", func() {
			var (
				// index             = []int{0, 1, 2, 3, 5, 7, 9, 11,13,15, 16, 17}
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
					UpperBound99ThIndex: 5,
				},
				{
					NominalIndex:        9,
					LowerBound99ThIndex: 7,
					UpperBound99ThIndex: 11,
				},
				{
					NominalIndex:        15,
					LowerBound99ThIndex: 13,
					UpperBound99ThIndex: 16,
				},
			}
			var merged []*Run
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
				StartHour:           time.Unix(3600, 0),
				EndHour:             time.Unix(3*3600, 0),
			})

			So(ibSegments[1], ShouldResembleProto, &Segment{
				StartIndex:                     3,
				EndIndex:                       8,
				HasStartChangepoint:            true,
				StartPosition:                  4,
				StartPositionLowerBound99Th:    3,
				StartPositionUpperBound99Th:    5,
				EndPosition:                    6,
				StartHour:                      time.Unix(4*3600, 0),
				EndHour:                        time.Unix(6*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(6*3600, 0),
			})

			So(ibSegments[2], ShouldResembleProto, &Segment{
				StartIndex:                     9,
				EndIndex:                       14,
				HasStartChangepoint:            true,
				StartPosition:                  7,
				StartPositionLowerBound99Th:    6,
				StartPositionUpperBound99Th:    8,
				EndPosition:                    9,
				StartHour:                      time.Unix(7*3600, 0),
				EndHour:                        time.Unix(9*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(9*3600, 0),
			})

			So(ibSegments[3], ShouldResembleProto, &Segment{
				StartIndex:                     15,
				EndIndex:                       17,
				HasStartChangepoint:            true,
				StartPosition:                  10,
				StartPositionLowerBound99Th:    9,
				StartPositionUpperBound99Th:    11,
				EndPosition:                    12,
				StartHour:                      time.Unix(10*3600, 0),
				EndHour:                        time.Unix(12*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(12*3600, 0),
			})
		})
	})
}

func TestEvictSegments(t *testing.T) {
	Convey("Does not evict if there is no buffer pressure and no changepoint", t, func() {
		ib := genInputBuffer(100, 2000, simpleVerdicts(100, 1, []int{}))
		segments := []*Segment{
			{
				StartIndex:    0,
				EndIndex:      99,
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
				StartIndex:          0,
				EndIndex:            2049,
				HasStartChangepoint: false,
				StartHour:           time.Unix(1*3600, 0),
				StartPosition:       1,
				EndHour:             time.Unix(2050*3600, 0),
				EndPosition:         2050,
			},
			{
				StartIndex:          2050,
				EndIndex:            2099,
				HasStartChangepoint: true,
				StartHour:           time.Unix(2051*3600, 0),
				StartPosition:       2051,
				EndHour:             time.Unix(2100*3600, 0),
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
			State:                          cpb.SegmentState_FINALIZING,
			HasStartChangepoint:            false,
			StartHour:                      time.Unix(1*3600, 0),
			StartPosition:                  1,
			MostRecentUnexpectedResultHour: time.Unix(51*3600, 0),
			Runs:                           copyAndUnflattenRuns(simpleVerdicts(100, 1, []int{50})),
		})

		So(remaining[0], ShouldResembleProto, &Segment{
			StartIndex:                     0,
			EndIndex:                       1949,
			StartPosition:                  101,
			StartHour:                      time.Unix(101*3600, 0),
			EndPosition:                    2050,
			EndHour:                        time.Unix(2050*3600, 0),
			MostRecentUnexpectedResultHour: time.Unix(1901*3600, 0),
		})

		So(remaining[1], ShouldResembleProto, &Segment{
			StartIndex:          1950,
			EndIndex:            1999,
			HasStartChangepoint: true,
			StartPosition:       2051,
			StartHour:           time.Unix(2051*3600, 0),
			EndPosition:         2100,
			EndHour:             time.Unix(2100*3600, 0),
		})
	})

	Convey("Evict finalized segment", t, func() {
		ib := genInputBuffer(100, 2000, simpleVerdicts(2100, 1, []int{}))
		segments := []*Segment{
			{
				StartIndex:          0, // Finalized segment.
				EndIndex:            39,
				HasStartChangepoint: false,
				StartHour:           time.Unix(1*3600, 0),
				StartPosition:       1,
				EndHour:             time.Unix(40*3600, 0),
				EndPosition:         40,
			},
			{
				StartIndex:                  40, // Finalized segment.
				EndIndex:                    79,
				HasStartChangepoint:         true,
				StartHour:                   time.Unix(41*3600, 0),
				StartPositionLowerBound99Th: 30,
				StartPositionUpperBound99Th: 50,
				StartPosition:               41,
				EndHour:                     time.Unix(80*3600, 0),
				EndPosition:                 80,
			},
			{
				StartIndex:                  80, // A finalizing segment.
				EndIndex:                    2049,
				HasStartChangepoint:         true,
				StartHour:                   time.Unix(81*3600, 0),
				StartPosition:               81,
				StartPositionLowerBound99Th: 70,
				StartPositionUpperBound99Th: 90,
				EndHour:                     time.Unix(2050*3600, 0),
				EndPosition:                 2050,
			},
			{
				StartIndex:          2050, // An active segment.
				EndIndex:            2099,
				HasStartChangepoint: true,
				StartHour:           time.Unix(2051*3600, 0),
				StartPosition:       2051,
				EndHour:             time.Unix(2100*3600, 0),
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
			State:               cpb.SegmentState_FINALIZED,
			HasStartChangepoint: false,
			StartHour:           time.Unix(1*3600, 0),
			StartPosition:       1,
			EndHour:             time.Unix(40*3600, 0),
			EndPosition:         40,
			Runs:                copyAndUnflattenRuns(simpleVerdicts(40, 1, []int{})),
		})

		So(evicted[1], ShouldResembleProto, EvictedSegment{
			State:                       cpb.SegmentState_FINALIZED,
			HasStartChangepoint:         true,
			StartHour:                   time.Unix(41*3600, 0),
			StartPosition:               41,
			StartPositionLowerBound99Th: 30,
			StartPositionUpperBound99Th: 50,
			EndHour:                     time.Unix(80*3600, 0),
			EndPosition:                 80,
			Runs:                        copyAndUnflattenRuns(simpleVerdicts(40, 41, []int{})),
		})

		So(evicted[2], ShouldResembleProto, EvictedSegment{
			State:                       cpb.SegmentState_FINALIZING,
			HasStartChangepoint:         true,
			StartHour:                   time.Unix(81*3600, 0),
			StartPosition:               81,
			StartPositionLowerBound99Th: 70,
			StartPositionUpperBound99Th: 90,
			Runs:                        copyAndUnflattenRuns(simpleVerdicts(20, 81, []int{})),
		})

		So(remaining[0], ShouldResembleProto, &Segment{
			StartIndex:    0,
			StartPosition: 101,
			StartHour:     time.Unix(101*3600, 0),
			EndIndex:      1949,
			EndPosition:   2050,
			EndHour:       time.Unix(2050*3600, 0),
		})

		So(remaining[1], ShouldResembleProto, &Segment{
			StartIndex:          1950,
			EndIndex:            1999,
			HasStartChangepoint: true,
			StartPosition:       2051,
			StartHour:           time.Unix(2051*3600, 0),
			EndPosition:         2100,
			EndHour:             time.Unix(2100*3600, 0),
		})
	})

	Convey("Evict all hot buffer", t, func() {
		ib := genInputBuffer(100, 2000, simpleVerdicts(2000, 1, []int{}))
		ib.HotBuffer = History{
			Runs: []Run{
				{
					CommitPosition: 10,
				},
			},
		}
		segments := []*Segment{
			{
				StartIndex:          0, // Finalized segment.
				EndIndex:            39,
				HasStartChangepoint: false,
				StartHour:           time.Unix(1*3600, 0),
				StartPosition:       1,
				EndHour:             time.Unix(39*3600, 0),
				EndPosition:         39,
			},
			{
				StartIndex:                  40, // A finalizing segment.
				EndIndex:                    2000,
				HasStartChangepoint:         true,
				StartHour:                   time.Unix(40*3600, 0),
				StartPosition:               40,
				StartPositionLowerBound99Th: 30,
				StartPositionUpperBound99Th: 50,
				EndHour:                     time.Unix(2000*3600, 0),
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
		So(len(sib.InputBuffer.HotBuffer.Runs), ShouldEqual, 0)
		So(len(sib.InputBuffer.ColdBuffer.Runs), ShouldEqual, 1961)

		expectedRuns := []Run{
			// The run in the hot buffer.
			{CommitPosition: 10},
		}
		// Plus the evicted runs in the cold buffer.
		expectedRuns = append(expectedRuns, simpleVerdicts(39, 1, []int{})...)
		sort.Slice(expectedRuns, func(i, j int) bool {
			ri, rj := expectedRuns[i], expectedRuns[j]
			if ri.CommitPosition != rj.CommitPosition {
				return ri.CommitPosition < rj.CommitPosition
			}
			return ri.Hour.Before(rj.Hour)
		})

		So(evicted[0], ShouldResembleProto, EvictedSegment{
			State:               cpb.SegmentState_FINALIZED,
			HasStartChangepoint: false,
			StartHour:           time.Unix(1*3600, 0),
			StartPosition:       1,
			EndHour:             time.Unix(39*3600, 0),
			EndPosition:         39,
			Runs:                copyAndUnflattenRuns(expectedRuns),
		})

		So(evicted[1], ShouldResembleProto, EvictedSegment{
			State:                       cpb.SegmentState_FINALIZING,
			HasStartChangepoint:         true,
			StartHour:                   time.Unix(40*3600, 0),
			StartPosition:               40,
			StartPositionLowerBound99Th: 30,
			StartPositionUpperBound99Th: 50,
			Runs:                        []*Run{},
		})

		So(remaining[0], ShouldResembleProto, segments[1])
	})
}

func simpleVerdicts(verdictCount int, startPos int, unexpectedIndices []int) []Run {
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

func genInputBuffer(hotCap int, coldCap int, history []Run) *Buffer {
	return &Buffer{
		HotBufferCapacity:  hotCap,
		ColdBufferCapacity: coldCap,
		HotBuffer:          History{},
		ColdBuffer: History{
			Runs: history,
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
			Runs: history,
		},
	}
}
