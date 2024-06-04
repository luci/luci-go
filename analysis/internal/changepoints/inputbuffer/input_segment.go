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
	"time"

	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
)

// Segment is a representation of segments in input buffer.
// It is only use in-memory. It will not be stored in spanner or bigquery.
type Segment struct {
	// Start index in the input buffer history, inclusively.
	// As in the history slice, runs are store oldest first, so StartIndex
	// corresponds to the oldest run in the segment.
	StartIndex int
	// End index in the input buffer history, inclusively.
	// As in the history slice, runs are stored oldest first, so EndIndex
	// corresponds to the newest run in the segment.
	EndIndex int

	// The following fields are copied from the Segment proto.

	// Whether the segment is the first segment in the input buffer.
	HasStartChangepoint bool
	// The earliest commit position included in the segment.
	StartPosition int64
	// The earliest hour a run with the given StartPosition was recorded.
	StartHour time.Time
	// The end commit position of the segment. Always set.
	// The invariant EndPosition >= StartPosition holds.
	EndPosition int64
	// The latest hour a run with the last commit position in the segment
	// was recorded.
	EndHour time.Time
	// The lower bound of the change point position at the start of the segment
	// in a 99% two-tailed confidence interval. Inclusive.
	// Only set if HasStartChangepoint is set. If set, the invariant
	// previous_segment.StartPosition <= StartPositionLowerBound99Th <= StartPosition.
	StartPositionLowerBound99Th int64
	// The upper bound of the change point position at the start of the segment
	// in a 99% two-tailed confidence interval. Inclusive.
	// Only set if HasStartChangepoint is set. If set, the invariant
	// StartPosition <= StartPositionUpperBound99Th <= EndPosition
	// holds.
	StartPositionUpperBound99Th int64

	// The hour the most recent run with an unexpected test result
	// was produced.
	MostRecentUnexpectedResultHour time.Time
}

func (s *Segment) Length() int {
	return s.EndIndex - s.StartIndex + 1
}

// EvictedSegment represents a segment or segment part which was evicted
// from the input buffer.
//
// A segment may be partially evicted for one or both of the following reasons:
//   - The eviction is occuring because of limited input buffer space
//     (not because of a finalized changepoint), so only a fraction
//     of the segment needs to be evicted.
//   - Previously, part of the segment was evicted (for the above
//     reason), so subsequent evictions are necessarily only
//     in relation to the remaining part of that segment.
//
// The consumer generally does not need to be concerned about which
// of these cases applies, and should always process evicted segments
// in commit position order, merging them with any previously
// evicted finalizing segment in the output buffer (if any) (but not
// any previously evicted FINALIZED segment).
type EvictedSegment struct {
	// The state of the segment. Will be FINALIZING or FINALIZED.
	State cpb.SegmentState
	// Whether the segment is the first segment in the input buffer.
	HasStartChangepoint bool
	// The earliest commit position included in the segment.
	StartPosition int64
	// The earliest hour a run with the given start_position was recorded.
	StartHour time.Time
	// The end commit position of the segment.
	// Only set on FINALIZED segments where the end position is known.
	// If set, the invariant EndPosition >= StartPosition holds.
	EndPosition int64
	// The latest hour a run with the last commit position in the segment
	// was recorded.
	// Only set if EndPosition is set.
	EndHour time.Time
	// The lower bound of the change point position at the start of the segment
	// in a 99% two-tailed confidence interval. Inclusive.
	// Only set if HasStartChangepoint is set. If set, the invariant
	// previous_segment.StartPosition <= StartPositionLowerBound99Th <= StartPosition.
	StartPositionLowerBound99Th int64
	// The upper bound of the change point position at the start of the segment
	// in a 99% two-tailed confidence interval. Inclusive.
	// Only set if HasStartChangepoint is set. If set, the invariant
	// StartPosition <= StartPositionUpperBound99Th <= EndPosition
	// holds.
	StartPositionUpperBound99Th int64
	// The hour the most recent run with an unexpected test result
	// was produced.
	MostRecentUnexpectedResultHour time.Time
	// The runs which are being evicted. These correspond to the
	// Segment above. Runs are ordered oldest commit position first,
	// then oldest hour first.
	Runs []*Run
}

// SegmentedInputBuffer wraps the input buffer and the segments it contains.
type SegmentedInputBuffer struct {
	InputBuffer *Buffer
	// The Segments are disjoint and are sorted by StartIndex ascendingly.
	Segments []*Segment
}

// ChangePoint records the index position of a change point, together with its
// confidence interval.
type ChangePoint struct {
	// NominalIndex is nominal index of the change point in history.
	NominalIndex int
	// LowerBound99ThIndex and UpperBound99ThIndex are indices (in history) of
	// the 99% confidence interval of the change point.
	LowerBound99ThIndex int
	UpperBound99ThIndex int
}

// Segmentize generates segments based on the input buffer and
// the change points detected.
// Input buffer runs are sorted by commit position (oldest first), then
// by result time (oldest first) and MUST have been returned by a call to
// MergeBuffer(...) immediately prior to this Segmentize call (i.e. without
// mutating the input buffer or the merge buffer.)
// changePoints is the change points for history. It is
// sorted in ascending order (smallest index first).
func (ib *Buffer) Segmentize(history []*Run, changePoints []ChangePoint) *SegmentedInputBuffer {
	// Exit early if we have empty history.
	if len(history) == 0 {
		return &SegmentedInputBuffer{
			InputBuffer: ib,
			Segments:    []*Segment{},
		}
	}

	segments := make([]*Segment, len(changePoints)+1)
	// Go from back to front, for easier processing of the confidence interval.
	segmentEndIndex := len(history) - 1
	for i := len(changePoints) - 1; i >= 0; i-- {
		// Add the segment starting from change point.
		changePoint := changePoints[i]
		segmentStartIndex := changePoint.NominalIndex
		sw := inputBufferSegment(segmentStartIndex, segmentEndIndex, history)
		sw.HasStartChangepoint = true
		sw.StartPositionLowerBound99Th = history[changePoint.LowerBound99ThIndex].CommitPosition
		sw.StartPositionUpperBound99Th = history[changePoint.UpperBound99ThIndex].CommitPosition
		segments[i+1] = sw
		segmentEndIndex = segmentStartIndex - 1
	}

	// Add the first segment.
	sw := inputBufferSegment(0, segmentEndIndex, history)
	segments[0] = sw

	return &SegmentedInputBuffer{
		InputBuffer: ib,
		Segments:    segments,
	}
}

// inputBufferSegment returns a Segment from startIndex (inclusively) to
// endIndex (inclusively).
func inputBufferSegment(startIndex, endIndex int, history []*Run) *Segment {
	if startIndex > endIndex {
		panic("invalid segment index: startIndex > endIndex")
	}
	return &Segment{
		StartIndex:                     startIndex,
		EndIndex:                       endIndex,
		StartPosition:                  int64(history[startIndex].CommitPosition),
		EndPosition:                    int64(history[endIndex].CommitPosition),
		StartHour:                      history[startIndex].Hour,
		EndHour:                        history[endIndex].Hour,
		MostRecentUnexpectedResultHour: mostRecentUnexpectedResultHour(history[startIndex : endIndex+1]),
	}
}

// EvictSegments evicts segments from the segmented input buffer.
//
// Returned EvictedSegments are sorted from the oldest commit position
// to the newest.
//
// A segment will be evicted if:
//  1. The changepoint that ends the segment has been finalized,
//     because half of the input buffer is newer than the ending commit
//     position). In this case, the entire remainder of the segment will
//     be evicted.
//  2. There is storage pressure in the input buffer (it is at risk of
//     containing too many runs). In this case, a segment will be
//     partially evicted, and that segment will be 'finalizing'.
//
// Note that if the last segment evicted is a finalized segment, this function
// will add an extra finalizing segment to the end of evicted segments. This is
// to keep track of the confidence interval of the starting commit position of
// the segment after the finalized segment. It is needed because after a
// finalized segment is evicted, its runs disappear from the input buffer
// and we can no longer calculate the confidence interval of the start of the
// next segment.
//
// As a result, the result of this function will contain all finalized segments,
// except for the last segment (if any), which is finalizing.
//
// The segments remaining after eviction will be in sib.Segments.
//
// If eviction occurs, the underlying input buffer will be modified and any
// cached versions of it (e.g. merged input buffer histories) should be treated
// as invalid.
func (sib *SegmentedInputBuffer) EvictSegments() []EvictedSegment {
	evictedSegments := []EvictedSegment{}
	remainingSegments := []*Segment{}

	// Evict finalized segments.
	segmentIndex := 0
	for ; segmentIndex < len(sib.Segments); segmentIndex++ {
		inputSegment := sib.Segments[segmentIndex]

		if inputSegment.StartIndex != 0 {
			panic("logic error: oldest segment does not have start index 0")
		}
		if !sib.InputBuffer.isSegmentFinalized(inputSegment) {
			break
		}
		seg := sib.InputBuffer.evictFinalizedSegment(inputSegment)
		evictedSegments = append(evictedSegments, seg)

		for j := segmentIndex + 1; j < len(sib.Segments); j++ {
			// Update subsequent segments to reflect the eviction.
			inputSegment := sib.Segments[j]
			inputSegment.EndIndex -= len(seg.Runs)
			inputSegment.StartIndex -= len(seg.Runs)
		}
	}

	// If the buffer is full, evict part of it to the finalizing segment.
	shouldEvict, endPos := sib.InputBuffer.EvictionRange()
	if shouldEvict {
		inputSegment := sib.Segments[segmentIndex]

		evicted, remaining := sib.InputBuffer.evictFinalizingSegment(endPos, inputSegment)
		evictedSegments = append(evictedSegments, evicted)
		remainingSegments = append(remainingSegments, remaining)

		// Update subsequent segments to reflect the eviction.
		for j := segmentIndex + 1; j < len(sib.Segments); j++ {
			inputSegment := sib.Segments[j]
			inputSegment.StartIndex -= len(evicted.Runs)
			inputSegment.EndIndex -= len(evicted.Runs)
		}
		segmentIndex++
	}

	// The remaining segments are active segments.
	remainingSegments = append(remainingSegments, sib.Segments[segmentIndex:]...)

	sib.Segments = remainingSegments

	// If the last segment is finalized, we also add a finalizing segment
	// to the end of the evicted segments, to record the start position
	// (and confidence interval) of the following segment.
	l := len(evictedSegments)
	if l > 0 && evictedSegments[l-1].State == cpb.SegmentState_FINALIZED {
		firstRemainingSeg := remainingSegments[0]
		evictedSegments = append(evictedSegments, EvictedSegment{
			State:                       cpb.SegmentState_FINALIZING,
			HasStartChangepoint:         true,
			StartPosition:               firstRemainingSeg.StartPosition,
			StartHour:                   firstRemainingSeg.StartHour,
			StartPositionLowerBound99Th: firstRemainingSeg.StartPositionLowerBound99Th,
			StartPositionUpperBound99Th: firstRemainingSeg.StartPositionUpperBound99Th,
			Runs:                        []*Run{},
		})
	}
	return evictedSegments
}

// isSegmentFinalized returns true if the segment is finalized, i.e.
// the ending commit position of the segment is in the oldest half of the
// buffer.
// It means not much refinement can be made to the segment.
func (ib *Buffer) isSegmentFinalized(seg *Segment) bool {
	capacity := ib.HotBufferCapacity + ib.ColdBufferCapacity
	// The number of runs which have commit positions newer than the segment.
	// Note that runs are stored in the input buffer from oldest to newest,
	// so those after seg.EndIndex are newer than the segment.
	runsNewerThanSegment := (ib.Size() - (seg.EndIndex + 1))
	return runsNewerThanSegment >= (capacity / 2)
}

// evictFinalizedSegment removes all runs of segment from input buffer.
// This has an assumption that the segment runs are at the beginning
// of the hot and cold buffers.
// Returns a segment containing the information about the runs being evicted.
func (ib *Buffer) evictFinalizedSegment(seg *Segment) EvictedSegment {
	if seg.StartIndex != 0 {
		panic("may only evict the oldest segment in the buffer")
	}

	// Evict hot buffer.
	evictEndIndex := -1
	for i, v := range ib.HotBuffer.Runs {
		if v.CommitPosition <= seg.EndPosition {
			evictEndIndex = i
		} else {
			break
		}
	}
	var evictedHotRuns []Run
	// EvictBefore(...) will modify the Runs slice in-place, we should
	// copy runs to a new slice to avoid them being overwritten.
	evictedHotRuns = append(evictedHotRuns, ib.HotBuffer.Runs[:evictEndIndex+1]...)

	ib.HotBuffer.EvictBefore(evictEndIndex + 1)

	// Evict cold buffer.
	evictEndIndex = -1
	for i, v := range ib.ColdBuffer.Runs {
		if v.CommitPosition <= seg.EndPosition {
			evictEndIndex = i
		} else {
			break
		}
	}
	var evictedColdRuns []Run
	if evictEndIndex > -1 {
		ib.IsColdBufferDirty = true
		// EvictBefore(...) will modify the Runs slice in-place, we should
		// copy runs to a new slice to avoid them being overwritten.
		evictedColdRuns = append(evictedColdRuns, ib.ColdBuffer.Runs[:evictEndIndex+1]...)
		ib.ColdBuffer.EvictBefore(evictEndIndex + 1)
	}

	var evictedRuns []*Run
	MergeOrderedRuns(evictedHotRuns, evictedColdRuns, &evictedRuns)

	// Return evicted segment.
	return EvictedSegment{
		State:                          cpb.SegmentState_FINALIZED,
		HasStartChangepoint:            seg.HasStartChangepoint,
		StartPosition:                  seg.StartPosition,
		StartHour:                      seg.StartHour,
		EndPosition:                    seg.EndPosition,
		EndHour:                        seg.EndHour,
		StartPositionLowerBound99Th:    seg.StartPositionLowerBound99Th,
		StartPositionUpperBound99Th:    seg.StartPositionUpperBound99Th,
		MostRecentUnexpectedResultHour: seg.MostRecentUnexpectedResultHour,
		Runs:                           evictedRuns,
	}
}

// evictFinalizingSegment evicts part of the finalizing segment when
// there is space pressure in the input buffer.
//
// All runs at index 0 to endPos (inclusive) are evicted.
//
// Note that space pressure is defined by the cold buffer meeting
// capacity and can only occur after a compaction from the hot buffer
// to the cold buffer (i.e. the hot buffer is empty and the cold buffer
// overflows).
// Returns evicted and remaining segments.
func (ib *Buffer) evictFinalizingSegment(endPos int, seg *Segment) (evicted EvictedSegment, remaining *Segment) {
	if len(ib.HotBuffer.Runs) > 0 {
		// This indicates a logic error.
		panic("hot buffer is not empty during eviction")
	}
	if seg.StartIndex != 0 {
		panic("may only evict from the oldest segment in the buffer")
	}
	if endPos >= seg.EndIndex-1 {
		// The whole segment should have been finalized and evicted if its end index was
		// in the older half of the buffer. Less than the whole old half of the buffer
		// is evicted due to space pressure at a time.
		panic("at least one run should be left in segment if evicting due to space pressure")
	}

	// EvictBefore(...) will modify the Runs in-place, we should
	// copy runs to a new slice to avoid them being overwritten.
	evictedRuns := copyAndUnflattenRuns(ib.ColdBuffer.Runs[:endPos+1])

	// Note: The retained runs are: ib.ColdBuffer.Runs[endPos+1 : seg.EndIndex+1].
	// After EvictBefore, they are ib.ColdBuffer.Runs[0 : (seg.EndIndex+1)-(endPos+1)]
	// (as we evict from the front of the buffer, where the oldest runs by commit position
	// are).

	ib.ColdBuffer.EvictBefore(endPos + 1)
	ib.IsColdBufferDirty = true
	// Evicted segment.
	evicted = EvictedSegment{
		State:                          cpb.SegmentState_FINALIZING,
		HasStartChangepoint:            seg.HasStartChangepoint,
		StartPosition:                  seg.StartPosition,
		StartHour:                      seg.StartHour,
		StartPositionLowerBound99Th:    seg.StartPositionLowerBound99Th,
		StartPositionUpperBound99Th:    seg.StartPositionUpperBound99Th,
		MostRecentUnexpectedResultHour: mostRecentUnexpectedResultHour(evictedRuns),
		Runs:                           evictedRuns,
	}

	// Before eviction, the retained runs were: ib.ColdBuffer.Runs[endPos+1 : seg.EndIndex+1].
	// After EvictBefore, they are ib.ColdBuffer.Runs[0 : (seg.EndIndex+1)-(endPos+1)]
	remainingRuns := ib.ColdBuffer.Runs[0 : (seg.EndIndex+1)-(endPos+1)]

	// Remaining segment.
	remaining = &Segment{
		StartIndex:                     0,
		StartPosition:                  remainingRuns[0].CommitPosition,
		StartHour:                      remainingRuns[0].Hour,
		EndIndex:                       len(remainingRuns) - 1,
		EndPosition:                    remainingRuns[len(remainingRuns)-1].CommitPosition,
		EndHour:                        remainingRuns[len(remainingRuns)-1].Hour,
		MostRecentUnexpectedResultHour: mostRecentUnexpectedResultHour(copyAndUnflattenRuns(remainingRuns)),
	}

	return evicted, remaining
}

// mostRecentUnexpectedResultHour return the hour for the most recent
// run that contains unexpected result.
// If no such run was found, returns the zero time.
func mostRecentUnexpectedResultHour(history []*Run) time.Time {
	var latest time.Time

	// history is sorted by commit position, not hour, so we need to do a loop.
	for _, run := range history {
		if run.Unexpected.Count() > 0 {
			if run.Hour.After(latest) {
				latest = run.Hour
			}
		}
	}
	return latest
}
