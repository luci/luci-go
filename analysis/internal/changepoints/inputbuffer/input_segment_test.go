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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/changepoints/model"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
)

func TestSegmentizeInputBuffer(t *testing.T) {
	ftt.Run("Segmentize input buffer", t, func(t *ftt.Test) {
		t.Run("No change point", func(t *ftt.Test) {
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
			assert.Loosely(t, len(ibSegments), should.Equal(1))
			assert.Loosely(t, ibSegments[0], should.Match(&Segment{
				StartIndex:                     0,
				EndIndex:                       8, // runs
				HasStartChangepoint:            false,
				StartPosition:                  1,
				EndPosition:                    6,
				StartHour:                      time.Unix(3600, 0),
				EndHour:                        time.Unix(6*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(4*3600, 0),
			}))
		})

		t.Run("With change points and retries", func(t *ftt.Test) {
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
					NominalIndex:         3,
					PositionDistribution: model.SimpleDistribution(3, 2),
				},
				{
					NominalIndex:         9,
					PositionDistribution: model.SimpleDistribution(9, 2),
				},
				{
					NominalIndex:         15,
					PositionDistribution: model.SimpleDistribution(15, 2),
				},
			}
			var merged []*Run
			ib.MergeBuffer(&merged)
			sib := ib.Segmentize(merged, cps)
			ibSegments := sib.Segments
			assert.Loosely(t, len(ibSegments), should.Equal(4))
			assert.Loosely(t, ibSegments[0], should.Match(&Segment{
				StartIndex:          0,
				EndIndex:            2,
				HasStartChangepoint: false,
				StartPosition:       1,
				EndPosition:         3,
				StartHour:           time.Unix(3600, 0),
				EndHour:             time.Unix(3*3600, 0),
			}))

			assert.Loosely(t, ibSegments[1], should.Match(&Segment{
				StartIndex:                     3,
				EndIndex:                       8,
				HasStartChangepoint:            true,
				StartPosition:                  4,
				StartPositionDistribution:      model.SimpleDistribution(3, 2),
				EndPosition:                    6,
				StartHour:                      time.Unix(4*3600, 0),
				EndHour:                        time.Unix(6*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(6*3600, 0),
			}))

			assert.Loosely(t, ibSegments[2], should.Match(&Segment{
				StartIndex:                     9,
				EndIndex:                       14,
				HasStartChangepoint:            true,
				StartPosition:                  7,
				StartPositionDistribution:      model.SimpleDistribution(9, 2),
				EndPosition:                    9,
				StartHour:                      time.Unix(7*3600, 0),
				EndHour:                        time.Unix(9*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(9*3600, 0),
			}))

			assert.Loosely(t, ibSegments[3], should.Match(&Segment{
				StartIndex:                     15,
				EndIndex:                       17,
				HasStartChangepoint:            true,
				StartPosition:                  10,
				StartPositionDistribution:      model.SimpleDistribution(15, 2),
				EndPosition:                    12,
				StartHour:                      time.Unix(10*3600, 0),
				EndHour:                        time.Unix(12*3600, 0),
				MostRecentUnexpectedResultHour: time.Unix(12*3600, 0),
			}))
		})
	})
}

func TestEvictSegments(t *testing.T) {
	ftt.Run("Does not evict if there is no buffer pressure and no changepoint", t, func(t *ftt.Test) {
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
		assert.Loosely(t, len(evicted), should.BeZero)
		assert.Loosely(t, len(remaining), should.Equal(1))
		assert.Loosely(t, ib.IsColdBufferDirty, should.BeFalse)
		assert.Loosely(t, remaining[0], should.Match(segments[0]))
	})

	ftt.Run("Evict finalizing segment", t, func(t *ftt.Test) {
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
		assert.Loosely(t, len(evicted), should.Equal(1))
		assert.Loosely(t, len(remaining), should.Equal(2))
		assert.Loosely(t, ib.IsColdBufferDirty, should.BeTrue)

		assert.Loosely(t, evicted[0], should.Match(EvictedSegment{
			State:                          cpb.SegmentState_FINALIZING,
			HasStartChangepoint:            false,
			StartHour:                      time.Unix(1*3600, 0),
			StartPosition:                  1,
			MostRecentUnexpectedResultHour: time.Unix(51*3600, 0),
			Runs:                           copyAndUnflattenRuns(simpleVerdicts(100, 1, []int{50})),
		}))

		assert.Loosely(t, remaining[0], should.Match(&Segment{
			StartIndex:                     0,
			EndIndex:                       1949,
			StartPosition:                  101,
			StartHour:                      time.Unix(101*3600, 0),
			EndPosition:                    2050,
			EndHour:                        time.Unix(2050*3600, 0),
			MostRecentUnexpectedResultHour: time.Unix(1901*3600, 0),
		}))

		assert.Loosely(t, remaining[1], should.Match(&Segment{
			StartIndex:          1950,
			EndIndex:            1999,
			HasStartChangepoint: true,
			StartPosition:       2051,
			StartHour:           time.Unix(2051*3600, 0),
			EndPosition:         2100,
			EndHour:             time.Unix(2100*3600, 0),
		}))
	})

	ftt.Run("Evict finalized segment", t, func(t *ftt.Test) {
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
				StartIndex:                40, // Finalized segment.
				EndIndex:                  79,
				HasStartChangepoint:       true,
				StartHour:                 time.Unix(41*3600, 0),
				StartPositionDistribution: model.SimpleDistribution(40, 10),
				StartPosition:             41,
				EndHour:                   time.Unix(80*3600, 0),
				EndPosition:               80,
			},
			{
				StartIndex:                80, // A finalizing segment.
				EndIndex:                  2049,
				HasStartChangepoint:       true,
				StartHour:                 time.Unix(81*3600, 0),
				StartPosition:             81,
				StartPositionDistribution: model.SimpleDistribution(80, 10),
				EndHour:                   time.Unix(2050*3600, 0),
				EndPosition:               2050,
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
		assert.Loosely(t, len(evicted), should.Equal(3))
		assert.Loosely(t, len(remaining), should.Equal(2))
		assert.Loosely(t, ib.IsColdBufferDirty, should.BeTrue)

		assert.Loosely(t, evicted[0], should.Match(EvictedSegment{
			State:               cpb.SegmentState_FINALIZED,
			HasStartChangepoint: false,
			StartHour:           time.Unix(1*3600, 0),
			StartPosition:       1,
			EndHour:             time.Unix(40*3600, 0),
			EndPosition:         40,
			Runs:                copyAndUnflattenRuns(simpleVerdicts(40, 1, []int{})),
		}))

		assert.Loosely(t, evicted[1], should.Match(EvictedSegment{
			State:                     cpb.SegmentState_FINALIZED,
			HasStartChangepoint:       true,
			StartHour:                 time.Unix(41*3600, 0),
			StartPosition:             41,
			StartPositionDistribution: model.SimpleDistribution(40, 10),
			EndHour:                   time.Unix(80*3600, 0),
			EndPosition:               80,
			Runs:                      copyAndUnflattenRuns(simpleVerdicts(40, 41, []int{})),
		}))

		assert.Loosely(t, evicted[2], should.Match(EvictedSegment{
			State:                     cpb.SegmentState_FINALIZING,
			HasStartChangepoint:       true,
			StartHour:                 time.Unix(81*3600, 0),
			StartPosition:             81,
			StartPositionDistribution: model.SimpleDistribution(80, 10),
			Runs:                      copyAndUnflattenRuns(simpleVerdicts(20, 81, []int{})),
		}))

		assert.Loosely(t, remaining[0], should.Match(&Segment{
			StartIndex:    0,
			StartPosition: 101,
			StartHour:     time.Unix(101*3600, 0),
			EndIndex:      1949,
			EndPosition:   2050,
			EndHour:       time.Unix(2050*3600, 0),
		}))

		assert.Loosely(t, remaining[1], should.Match(&Segment{
			StartIndex:          1950,
			EndIndex:            1999,
			HasStartChangepoint: true,
			StartPosition:       2051,
			StartHour:           time.Unix(2051*3600, 0),
			EndPosition:         2100,
			EndHour:             time.Unix(2100*3600, 0),
		}))
	})

	ftt.Run("Evict all hot buffer", t, func(t *ftt.Test) {
		ib := genInputBuffer(100, 2000, simpleVerdicts(2000, 1, []int{}))
		ib.HotBuffer = History{
			Runs: []Run{
				{
					SourcePosition: 10,
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
				StartIndex:                40, // A finalizing segment.
				EndIndex:                  2000,
				HasStartChangepoint:       true,
				StartHour:                 time.Unix(40*3600, 0),
				StartPosition:             40,
				StartPositionDistribution: model.SimpleDistribution(40, 10),
				EndHour:                   time.Unix(2000*3600, 0),
				EndPosition:               2000,
			},
		}

		sib := &SegmentedInputBuffer{
			InputBuffer: ib,
			Segments:    segments,
		}
		evicted := sib.EvictSegments()
		remaining := sib.Segments
		assert.Loosely(t, len(evicted), should.Equal(2))
		assert.Loosely(t, len(remaining), should.Equal(1))
		assert.Loosely(t, sib.InputBuffer.IsColdBufferDirty, should.BeTrue)

		// Hot bufffer should be empty.
		assert.Loosely(t, len(sib.InputBuffer.HotBuffer.Runs), should.BeZero)
		assert.Loosely(t, len(sib.InputBuffer.ColdBuffer.Runs), should.Equal(1961))

		expectedRuns := []Run{
			// The run in the hot buffer.
			{SourcePosition: 10},
		}
		// Plus the evicted runs in the cold buffer.
		expectedRuns = append(expectedRuns, simpleVerdicts(39, 1, []int{})...)
		sort.Slice(expectedRuns, func(i, j int) bool {
			ri, rj := expectedRuns[i], expectedRuns[j]
			if ri.SourcePosition != rj.SourcePosition {
				return ri.SourcePosition < rj.SourcePosition
			}
			return ri.Hour.Before(rj.Hour)
		})

		assert.Loosely(t, evicted[0], should.Match(EvictedSegment{
			State:               cpb.SegmentState_FINALIZED,
			HasStartChangepoint: false,
			StartHour:           time.Unix(1*3600, 0),
			StartPosition:       1,
			EndHour:             time.Unix(39*3600, 0),
			EndPosition:         39,
			Runs:                copyAndUnflattenRuns(expectedRuns),
		}))

		assert.Loosely(t, evicted[1], should.Match(EvictedSegment{
			State:                     cpb.SegmentState_FINALIZING,
			HasStartChangepoint:       true,
			StartHour:                 time.Unix(40*3600, 0),
			StartPosition:             40,
			StartPositionDistribution: model.SimpleDistribution(40, 10),
			Runs:                      []*Run{},
		}))

		assert.Loosely(t, remaining[0], should.Match(segments[1]))
	})
}

func simpleVerdicts(verdictCount int, startPos int, unexpectedIndices []int) []Run {
	positions := make([]int, verdictCount)
	total := make([]int, verdictCount)
	hasUnexpected := make([]int, verdictCount)
	for i := range verdictCount {
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
