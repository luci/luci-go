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

	"google.golang.org/protobuf/types/known/timestamppb"

	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
)

// Segment is a representation of segments in input buffer.
// It is only use in-memory. It will not be stored in spanner or bigquery.
type Segment struct {
	// Start index in the input buffer history, inclusively.
	// As in the history slice, verdicts are store oldest first, so StartIndex
	// corresponds to the oldest verdict in the segment.
	StartIndex int
	// End index in the input buffer history, inclusively.
	// As in the history slice, verdicts are store oldest first, so EndIndex
	// corresponds to the newest verdict in the segment.
	EndIndex int
	// Counts the statistics of the segment.
	// Note that this includes all verdicts, as opposed to Segment.FinalizedCount
	// which only includes finalized verdicts.
	Counts *cpb.Counts
	// The hour the most recent verdict with an unexpected test result
	// was produced.
	// Note that this includes all verdicts, as opposed to Segment.FinalizedCount
	// which only includes finalized verdicts.
	MostRecentUnexpectedResultHourAllVerdicts *timestamppb.Timestamp

	// The following fields are copied from the Segment proto.

	// Whether the segment is the first segment in the input buffer.
	HasStartChangepoint bool
	// The earliest commit position included in the segment.
	StartPosition int64
	// The earliest hour a verdict with the given start_position was recorded.
	StartHour *timestamppb.Timestamp
	// The end commit position of the segment.
	// If set, the invariant end_position >= start_position holds.
	EndPosition int64
	// The latest hour a verdict with the last commit position in the segment
	// was recorded.
	EndHour *timestamppb.Timestamp
	// The lower bound of the change point position at the start of the segment
	// in a 99% two-tailed confidence interval. Inclusive.
	// Only set if has_start_changepoint is set. If set, the invariant
	// previous_segment.start_position <= start_position_lower_bound_99th <= start_position.
	StartPositionLowerBound99Th int64
	// The upper bound of the change point position at the start of the segment
	// in a 99% two-tailed confidence interval. Inclusive.
	// Only set if has_start_changepoint is set. If set, the invariant
	// start_position <= start_position_upper_bound_99th <= end_position
	// holds.
	StartPositionUpperBound99Th int64
}

func (s *Segment) Length() int {
	return s.EndIndex - s.StartIndex + 1
}

// EvictedSegment represents a segment or segment part which was evicted
// from the input buffer.
type EvictedSegment struct {
	// The segment (either full or partial) which is being evicted.
	// A segment may be partial for one or both of the following reasons:
	// - The eviction is occuring because of limited input buffer space
	//   (not because of a finalized changepoint), so only a fraction
	//   of the segment needs to be evicted.
	// - Previously, part of the segment was evicted (for the above
	//   reason), so subsequent evictions are necessarily only
	//   in relation to the remaining part of that segment.
	//
	// The consumer generally does not need to be concerned about which
	// of these cases applies, and should always process evicted segments
	// in commit position order, merging them with any previously
	// evicted finalizing segment (if any).
	Segment *cpb.Segment

	// The verdicts which are being evicted. These correspond to the
	// Segment above. Not in any particular order.
	Verdicts []PositionVerdict
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
// Input buffer verdicts are sorted by commit position (oldest first), then
// by result time (oldest first) and MUST have been returned by a call to
// MergeBuffer(...) immediately prior to this Segmentize call (i.e. without
// mutating the input buffer or the merge buffer.)
// changePoints is the change points for history. It is
// sorted in ascending order (smallest index first).
func (ib *Buffer) Segmentize(history []PositionVerdict, changePoints []ChangePoint) *SegmentedInputBuffer {
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
		sw.StartPositionLowerBound99Th = int64(history[changePoint.LowerBound99ThIndex].CommitPosition)
		sw.StartPositionUpperBound99Th = int64(history[changePoint.UpperBound99ThIndex].CommitPosition)
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
func inputBufferSegment(startIndex, endIndex int, history []PositionVerdict) *Segment {
	if startIndex > endIndex {
		panic("invalid segment index: startIndex > endIndex")
	}
	return &Segment{
		StartIndex:    startIndex,
		EndIndex:      endIndex,
		StartPosition: int64(history[startIndex].CommitPosition),
		EndPosition:   int64(history[endIndex].CommitPosition),
		StartHour:     timestamppb.New(history[startIndex].Hour),
		EndHour:       timestamppb.New(history[endIndex].Hour),
		Counts:        segmentCounts(history[startIndex : endIndex+1]),
		MostRecentUnexpectedResultHourAllVerdicts: mostRecentUnexpectedResultHour(history[startIndex : endIndex+1]),
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
//     containing too many verdicts). In this case, a segment will be
//     partially evicted, and that segment will be 'finalizing'.
//
// Note that if the last segment evicted is a finalized segment, this function
// will add an extra finalizing segment to the end of evicted segments. This is
// to keep track of the confidence interval of the starting commit position of
// the segment after the finalized segment. It is needed because after a
// finalized segment is evicted, its verdicts disappear from the input buffer
// and we can no longer calculate the confidence interval of the start of the
// next segment.
//
// As a result, the result of this function will contain all finalized segments,
// except for the last segment (if any), which is finalizing.
//
// The segments remaining after eviction will be in sib.Segments.
func (sib *SegmentedInputBuffer) EvictSegments() []EvictedSegment {
	evictedSegments := []EvictedSegment{}
	remainingSegments := []*Segment{}

	// Evict finalized segments.
	segmentIndex := 0
	for ; segmentIndex < len(sib.Segments); segmentIndex++ {
		inSeg := sib.Segments[segmentIndex]
		// Update the start and end index of inSeg.
		// Note that after eviction of previous finalized segments, inSeg is the
		// first remaining segment of the input buffer.
		inSeg.EndIndex -= inSeg.StartIndex
		inSeg.StartIndex = 0
		if !sib.InputBuffer.isSegmentFinalized(inSeg) {
			break
		}
		seg := sib.InputBuffer.evictFinalizedSegment(inSeg)
		evictedSegments = append(evictedSegments, seg)
	}

	// If the buffer is full, evict part of it to the finalizing segment.
	shouldEvict, endPos := sib.InputBuffer.EvictionRange()
	remainingLength := 0
	if shouldEvict {
		inSeg := sib.Segments[segmentIndex]
		evicted, remaining := sib.InputBuffer.evictFinalizingSegment(endPos, inSeg)
		evictedSegments = append(evictedSegments, evicted)
		remainingSegments = append(remainingSegments, remaining)
		remainingLength = remaining.Length()
		segmentIndex++
	}

	// The remaining segments are active segments.
	offset := 0
	if segmentIndex < len(sib.Segments) {
		offset = sib.Segments[segmentIndex].StartIndex - remainingLength
	}
	for ; segmentIndex < len(sib.Segments); segmentIndex++ {
		inSeg := sib.Segments[segmentIndex]
		// Offset the indices of the segment due to previously evicted segments.
		inSeg.StartIndex -= offset
		inSeg.EndIndex -= offset
		remainingSegments = append(remainingSegments, inSeg)
	}

	sib.Segments = remainingSegments

	// If the last segment is finalized, we also add a finalizing segment
	// to the end of the evicted segments, to record the start position
	// (and confidence interval) of the following segment.
	l := len(evictedSegments)
	if l > 0 && evictedSegments[l-1].Segment.State == cpb.SegmentState_FINALIZED {
		firstRemainingSeg := remainingSegments[0]
		evictedSegments = append(evictedSegments, EvictedSegment{
			Segment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				HasStartChangepoint:          true,
				StartPosition:                firstRemainingSeg.StartPosition,
				StartHour:                    firstRemainingSeg.StartHour,
				StartPositionLowerBound_99Th: firstRemainingSeg.StartPositionLowerBound99Th,
				StartPositionUpperBound_99Th: firstRemainingSeg.StartPositionUpperBound99Th,
				FinalizedCounts:              &cpb.Counts{},
			},
			Verdicts: []PositionVerdict{},
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
	// The number of verdicts which have commit positions newer than the segment.
	// Note that verdicts are stored in the input buffer from oldest to newest,
	// so those after seg.EndIndex are newer than the segment.
	verdictsNewerThanSegment := (ib.Size() - seg.EndIndex)
	return verdictsNewerThanSegment >= (capacity / 2)
}

// evictFinalizedSegment removes all verdicts of segment from input buffer.
// This has an assumption that the segment verdicts are at the beginning
// of the hot and cold buffers.
// Returns a segment containing the information about the verdicts being evicted.
func (ib *Buffer) evictFinalizedSegment(seg *Segment) EvictedSegment {
	// Evict hot buffer.
	evictEndIndex := -1
	for i, v := range ib.HotBuffer.Verdicts {
		if v.CommitPosition <= int(seg.EndPosition) {
			evictEndIndex = i
		} else {
			break
		}
	}
	var evictedVerdicts []PositionVerdict
	// EvictBefore(...) will modify the Verdicts in-place, we should
	// copy verdicts to a new slice to avoid them being overwritten.
	evictedVerdicts = append(evictedVerdicts, ib.HotBuffer.Verdicts[:evictEndIndex+1]...)

	ib.HotBuffer.EvictBefore(evictEndIndex + 1)

	// Evict cold buffer.
	evictEndIndex = -1
	for i, v := range ib.ColdBuffer.Verdicts {
		if v.CommitPosition <= int(seg.EndPosition) {
			evictEndIndex = i
		} else {
			break
		}
	}
	if evictEndIndex > -1 {
		ib.IsColdBufferDirty = true
		// EvictBefore(...) will modify the Verdicts in-place, we should
		// copy verdicts to a new slice to avoid them being overwritten.
		evictedVerdicts = append(evictedVerdicts, ib.ColdBuffer.Verdicts[:evictEndIndex+1]...)
		ib.ColdBuffer.EvictBefore(evictEndIndex + 1)
	}

	// Return evicted segment.
	segment := &cpb.Segment{
		State:                          cpb.SegmentState_FINALIZED,
		FinalizedCounts:                seg.Counts,
		HasStartChangepoint:            seg.HasStartChangepoint,
		StartPosition:                  seg.StartPosition,
		StartHour:                      seg.StartHour,
		EndPosition:                    seg.EndPosition,
		EndHour:                        seg.EndHour,
		StartPositionLowerBound_99Th:   seg.StartPositionLowerBound99Th,
		StartPositionUpperBound_99Th:   seg.StartPositionUpperBound99Th,
		MostRecentUnexpectedResultHour: seg.MostRecentUnexpectedResultHourAllVerdicts,
	}
	return EvictedSegment{
		Segment:  segment,
		Verdicts: evictedVerdicts,
	}
}

// evictFinalizingSegment evicts part of the finalizing segment when
// there is space pressure in the input buffer.
// Note that space pressure is defined by the cold buffer meeting
// capacity and can only occur after a compaction from the hot buffer
// to the cold buffer (i.e. the hot buffer is empty and the cold buffer
// overflows).
// Returns evicted and remaining segments.
func (ib *Buffer) evictFinalizingSegment(endPos int, seg *Segment) (evicted EvictedSegment, remaining *Segment) {
	if len(ib.HotBuffer.Verdicts) > 0 {
		// This indicates a logic error.
		panic("hot buffer is not empty during eviction")
	}

	remainingCount := segmentCounts(ib.ColdBuffer.Verdicts[endPos+1 : seg.EndIndex+1])
	evictedMostRecentHour := mostRecentUnexpectedResultHour(ib.ColdBuffer.Verdicts[:endPos+1])
	remainingMostRecentHour := mostRecentUnexpectedResultHour(ib.ColdBuffer.Verdicts[endPos+1 : seg.EndIndex+1])

	// EvictBefore(...) will modify the Verdicts in-place, we should
	// copy verdicts to a new slice to avoid them being overwritten.
	evictedVerdicts := append([]PositionVerdict(nil), ib.ColdBuffer.Verdicts[:endPos+1]...)
	evictedCount := segmentCounts(evictedVerdicts)
	ib.ColdBuffer.EvictBefore(endPos + 1)
	ib.IsColdBufferDirty = true
	// Evicted segment.
	evicted = EvictedSegment{
		Segment: &cpb.Segment{
			State:                          cpb.SegmentState_FINALIZING,
			FinalizedCounts:                evictedCount,
			HasStartChangepoint:            seg.HasStartChangepoint,
			StartPosition:                  seg.StartPosition,
			StartHour:                      seg.StartHour,
			StartPositionLowerBound_99Th:   seg.StartPositionLowerBound99Th,
			StartPositionUpperBound_99Th:   seg.StartPositionUpperBound99Th,
			MostRecentUnexpectedResultHour: evictedMostRecentHour,
		},
		Verdicts: evictedVerdicts,
	}

	// Remaining segment.
	remaining = &Segment{
		StartIndex:  0,
		EndIndex:    seg.EndIndex - endPos - 1,
		Counts:      remainingCount,
		EndPosition: seg.EndPosition,
		EndHour:     seg.EndHour,
		MostRecentUnexpectedResultHourAllVerdicts: remainingMostRecentHour,
	}

	return evicted, remaining
}

// segmentCount counts the statistics of history.
func segmentCounts(history []PositionVerdict) *cpb.Counts {
	counts := &cpb.Counts{}
	for _, verdict := range history {
		counts.TotalVerdicts++
		if verdict.IsSimpleExpectedPass {
			counts.TotalRuns++
			counts.TotalResults++
			counts.ExpectedPassedResults++
		} else {
			verdictHasExpectedResults := false
			verdictHasUnexpectedResults := false
			for _, run := range verdict.Details.Runs {
				// Verdict-level statistics.
				verdictHasExpectedResults = verdictHasExpectedResults || (run.Expected.Count() > 0)
				verdictHasUnexpectedResults = verdictHasUnexpectedResults || (run.Unexpected.Count() > 0)

				if run.IsDuplicate {
					continue
				}
				// Result-level statistics (ignores duplicate runs).
				counts.TotalResults += int64(run.Expected.Count() + run.Unexpected.Count())
				counts.UnexpectedResults += int64(run.Unexpected.Count())
				counts.ExpectedPassedResults += int64(run.Expected.PassCount)
				counts.ExpectedFailedResults += int64(run.Expected.FailCount)
				counts.ExpectedCrashedResults += int64(run.Expected.CrashCount)
				counts.ExpectedAbortedResults += int64(run.Expected.AbortCount)
				counts.UnexpectedPassedResults += int64(run.Unexpected.PassCount)
				counts.UnexpectedFailedResults += int64(run.Unexpected.FailCount)
				counts.UnexpectedCrashedResults += int64(run.Unexpected.CrashCount)
				counts.UnexpectedAbortedResults += int64(run.Unexpected.AbortCount)

				// Run-level statistics (ignores duplicate runs).
				counts.TotalRuns++
				// flaky run.
				isFlakyRun := run.Expected.Count() > 0 && run.Unexpected.Count() > 0
				if isFlakyRun {
					counts.FlakyRuns++
				}
				// unexpected unretried run.
				isUnexpectedUnretried := run.Unexpected.Count() == 1 && run.Expected.Count() == 0
				if isUnexpectedUnretried {
					counts.UnexpectedUnretriedRuns++
				}
				// unexpected after retries run.
				isUnexpectedAfterRetries := run.Unexpected.Count() > 1 && run.Expected.Count() == 0
				if isUnexpectedAfterRetries {
					counts.UnexpectedAfterRetryRuns++
				}
			}
			if verdictHasUnexpectedResults && !verdictHasExpectedResults {
				counts.UnexpectedVerdicts++
			}
			if verdictHasUnexpectedResults && verdictHasExpectedResults {
				counts.FlakyVerdicts++
			}
		}
	}
	return counts
}

// mostRecentUnexpectedResultHour return the hours for the most recent
// verdict that contains unexpected result.
func mostRecentUnexpectedResultHour(history []PositionVerdict) *timestamppb.Timestamp {
	latest := time.Unix(0, 0)
	found := false
	// history is sorted by commit position, not hour, so we need to do a loop.
	for _, verdict := range history {
		for _, run := range verdict.Details.Runs {
			if run.IsDuplicate {
				continue
			}
			if run.Unexpected.Count() > 0 {
				if verdict.Hour.Unix() > latest.Unix() {
					latest = verdict.Hour
					found = true
				}
				break
			}
		}
	}
	if !found {
		return nil
	}
	return timestamppb.New(latest)
}
