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

	changepointspb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	Counts *changepointspb.Counts
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
	// The lower bound of the changepoint position at the start of the segment
	// in a 99% two-tailed confidence interval. Inclusive.
	// Only set if has_start_changepoint is set. If set, the invariant
	// previous_segment.start_position <= start_position_lower_bound_99th <= start_position.
	StartPositionLowerBound99Th int64
	// The upper bound of the changepoint position at the start of the segment
	// in a 99% two-tailed confidence interval. Inclusive.
	// Only set if has_start_changepoint is set. If set, the invariant
	// start_position <= start_position_upper_bound_99th <= end_position
	// holds.
	StartPositionUpperBound99Th int64
}

func (s *Segment) Length() int {
	return s.EndIndex - s.StartIndex + 1
}

// SegmentedInputBuffer wraps the input buffer and the segments it contains.
type SegmentedInputBuffer struct {
	InputBuffer *Buffer
	// The Segments are disjoint and are sorted by StartIndex ascendingly.
	Segments []*Segment
}

// Segmentize generates segments based on the input buffer and
// the changepoints detected.
// history contains the input buffer verdicts (merged of hot and cold buffer).
// It is sorted by commit position (oldest first), then by result time (oldest
// first).
// changepointIndices is the indices of the changepoints for history. It is
// sorted in ascending order (smallest index first).
func (ib *Buffer) Segmentize(changepointIndices []int) *SegmentedInputBuffer {
	history := ib.MergeBuffer()
	segments := []*Segment{}
	segmentStartIndex := 0
	for i, changepointIndex := range changepointIndices {
		segmentEndIndex := changepointIndex - 1
		sw := inputBufferSegment(segmentStartIndex, segmentEndIndex, history)
		if i > 0 {
			sw.HasStartChangepoint = true
		}

		segments = append(segments, sw)
		segmentStartIndex = segmentEndIndex + 1
	}
	// Add the last segment.
	if segmentStartIndex < len(history) {
		sw := inputBufferSegment(segmentStartIndex, len(history)-1, history)
		if len(segments) > 0 {
			sw.HasStartChangepoint = true
		}
		segments = append(segments, sw)
	}
	return &SegmentedInputBuffer{
		InputBuffer: ib,
		Segments:    segments,
	}
}

// inputBufferSegment returns a Segment from startIndex (inclusively) to
// endIndex (inclusively).
func inputBufferSegment(startIndex, endIndex int, history []PositionVerdict) *Segment {
	// TODO (nqmtuan): Set confidence interval.
	return &Segment{
		StartIndex:    startIndex,
		EndIndex:      endIndex,
		StartPosition: int64(history[startIndex].CommitPosition),
		EndPosition:   int64(history[endIndex].CommitPosition),
		StartHour:     timestamppb.New(history[startIndex].Hour),
		EndHour:       timestamppb.New(history[endIndex].Hour),
		Counts:        segmentCounts(startIndex, endIndex, history),
		MostRecentUnexpectedResultHourAllVerdicts: mostRecentUnexpectedResultHour(startIndex, endIndex, history),
	}
}

// EvictSegments evicts segments from the segmented input buffer.
// The sib.Segments are sorted from the oldest commit position to the newest.
// A segment will be evicted if:
//  1. It is a finalized segment. In such case, it will be fully evicted.
//  2. It is a finalizing segment. In such case, it will be partially evicted.
//
// Returns the list of evicted segments. All segments will be finalized, except
// for the last segment, which may be finalizing or finalized.
// The remaining segments will be in sib.Segments.
func (sib *SegmentedInputBuffer) EvictSegments() []*changepointspb.Segment {
	evictedSegments := []*changepointspb.Segment{}
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
func (ib *Buffer) evictFinalizedSegment(seg *Segment) *changepointspb.Segment {
	for i, v := range ib.HotBuffer.Verdicts {
		if v.CommitPosition > int(seg.EndPosition) {
			ib.HotBuffer.Verdicts = ib.HotBuffer.Verdicts[i:]
			break
		}
	}
	for i, v := range ib.ColdBuffer.Verdicts {
		if v.CommitPosition > int(seg.EndPosition) {
			if i > 0 {
				ib.IsColdBufferDirty = true
			}
			ib.ColdBuffer.Verdicts = ib.ColdBuffer.Verdicts[i:]
			break
		}
	}
	result := &changepointspb.Segment{
		State:                          changepointspb.SegmentState_FINALIZED,
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
	return result
}

// evictFinalizingSegment evicts part of the finalizing segment when an
// overflow occurs.
// Note that an overflow can only occur after a compaction from the hot buffer
// to the cold buffer (i.e. the hot buffer is empty and the cold buffer
// overflows).
// Returns evicted and remaining segments.
func (ib *Buffer) evictFinalizingSegment(endPos int, seg *Segment) (evicted *changepointspb.Segment, remaining *Segment) {
	evictedCount := segmentCounts(0, endPos, ib.ColdBuffer.Verdicts)
	remainingCount := segmentCounts(endPos+1, seg.EndIndex, ib.ColdBuffer.Verdicts)
	evictedMostRecentHour := mostRecentUnexpectedResultHour(0, endPos, ib.ColdBuffer.Verdicts)
	remainingMostRecentHour := mostRecentUnexpectedResultHour(endPos+1, seg.EndIndex, ib.ColdBuffer.Verdicts)
	ib.ColdBuffer.Verdicts = ib.ColdBuffer.Verdicts[endPos+1:]
	ib.IsColdBufferDirty = true
	// Evicted segment.
	evicted = &changepointspb.Segment{
		State:                          changepointspb.SegmentState_FINALIZING,
		FinalizedCounts:                evictedCount,
		HasStartChangepoint:            seg.HasStartChangepoint,
		StartPosition:                  seg.StartPosition,
		StartHour:                      seg.StartHour,
		MostRecentUnexpectedResultHour: evictedMostRecentHour,
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

// segmentCount counts the statistics of segment from startIndex to endIndex in
// the input buffer history.
func segmentCounts(startIndex int, endIndex int, history []PositionVerdict) *changepointspb.Counts {
	counts := &changepointspb.Counts{}
	for i := startIndex; i <= endIndex; i++ {
		counts.TotalVerdicts++
		verdict := history[i]
		if verdict.IsSimpleExpected {
			counts.TotalRuns++
			counts.TotalResults++
		} else {
			verdictHasExpectedResults := false
			verdictHasUnexpectedResults := false
			for _, run := range verdict.Details.Runs {
				if run.IsDuplicate {
					continue
				}
				counts.TotalRuns++
				counts.TotalResults += int64(run.ExpectedResultCount + run.UnexpectedResultCount)
				counts.UnexpectedResults += int64(run.UnexpectedResultCount)

				// flaky run.
				isFlakyRun := run.ExpectedResultCount > 0 && run.UnexpectedResultCount > 0
				if isFlakyRun {
					counts.FlakyRuns++
				}
				// unexpected unretried run.
				isUnexpectedUnretried := run.UnexpectedResultCount == 1 && run.ExpectedResultCount == 0
				if isUnexpectedUnretried {
					counts.UnexpectedUnretriedRuns++
				}
				// unexpected after retries run.
				isUnexpectedAfterRetries := run.UnexpectedResultCount > 1 && run.ExpectedResultCount == 0
				if isUnexpectedAfterRetries {
					counts.UnexpectedAfterRetryRuns++
				}
				verdictHasExpectedResults = verdictHasExpectedResults || (run.ExpectedResultCount > 0)
				verdictHasUnexpectedResults = verdictHasUnexpectedResults || (run.UnexpectedResultCount > 0)
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
func mostRecentUnexpectedResultHour(startIndex int, endIndex int, history []PositionVerdict) *timestamppb.Timestamp {
	latest := time.Unix(0, 0)
	found := false
	// history is sorted by commit position, not hour, so we need to do a loop.
	for index := startIndex; index <= endIndex; index++ {
		pv := history[index]
		for _, run := range pv.Details.Runs {
			if run.IsDuplicate {
				continue
			}
			if run.UnexpectedResultCount > 0 {
				if pv.Hour.Unix() > latest.Unix() {
					latest = pv.Hour
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
