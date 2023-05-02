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
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	changepointspb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// TestVariantBranch represents one row in the TestVariantBranch spanner table.
// See go/luci-test-variant-analysis-design for details.
type TestVariantBranch struct {
	// IsNew is a boolean to denote if the TestVariantBranch is new or already
	// existed in Spanner.
	// It is used for reducing the number of mutations. For example, the Variant
	// field is only inserted once.
	IsNew                  bool
	Project                string
	TestID                 string
	VariantHash            string
	GitReferenceHash       []byte
	Variant                *pb.Variant
	InputBuffer            *inputbuffer.Buffer
	RecentChangepointCount int64
	// Store the finalizing segment, if any.
	// The count for the finalizing segment should only include the verdicts
	// that are not in the input buffer anymore.
	FinalizingSegment *changepointspb.Segment
	// Store all the finalized segments for the test variant branch.
	FinalizedSegments *changepointspb.Segments
	// If this is true, it means we should trigger a write of FinalizingSegment
	// to Spanner.
	IsFinalizingSegmentDirty bool
	// If this is true, it means we should trigger a write of FinalizedSegments
	// to Spanner.
	IsFinalizedSegmentsDirty bool
}

// InsertToInputBuffer inserts data of a new test variant into the input
// buffer.
func (tvb *TestVariantBranch) InsertToInputBuffer(pv inputbuffer.PositionVerdict) {
	tvb.InputBuffer.InsertVerdict(pv)
}

// InsertFinalizedSegment inserts a segment to the end of finalized segments.
func (tvb *TestVariantBranch) InsertFinalizedSegment(segment *changepointspb.Segment) {
	if tvb.FinalizedSegments == nil {
		tvb.FinalizedSegments = &changepointspb.Segments{}
	}
	// Assert that segment is finalized.
	if segment.State != changepointspb.SegmentState_FINALIZED {
		panic("insert non-finalized segment to FinalizedSegments")
	}
	// Assert that inserted segment is later than existing segments.
	l := len(tvb.FinalizedSegments.Segments)
	if l > 0 && tvb.FinalizedSegments.Segments[l-1].EndPosition >= segment.StartPosition {
		panic("insert older segment to FinalizedSegments")
	}
	tvb.FinalizedSegments.Segments = append(tvb.FinalizedSegments.Segments, segment)
	tvb.IsFinalizedSegmentsDirty = true
}

// UpdateOutputBuffer updates the output buffer with the evicted segments from
// the input buffer.
// evictedSegments contain all finalized segments, except for the last segment,
// which may be a finalizing or finalized segment.
// evictedSegments is sorted in ascending order of commit position (oldest
// segment first).
func (tvb *TestVariantBranch) UpdateOutputBuffer(evictedSegments []*changepointspb.Segment) {
	// Nothing to update.
	if len(evictedSegments) == 0 {
		return
	}
	verifyEvictedSegments(evictedSegments)
	// If there is a finalizing segment in the output buffer, this finalizing
	// segment should be "combined" with the first evicted segment.
	segmentIndex := 0
	if tvb.FinalizingSegment != nil {
		segmentIndex = 1
		combinedSegment := combineSegment(tvb.FinalizingSegment, evictedSegments[0])
		tvb.IsFinalizingSegmentDirty = true
		if combinedSegment.State == changepointspb.SegmentState_FINALIZING {
			// Replace the finalizing segment.
			tvb.FinalizingSegment = combinedSegment
		} else { // Finalized state.
			tvb.FinalizingSegment = nil
			tvb.InsertFinalizedSegment(combinedSegment)
		}
	}

	for ; segmentIndex < len(evictedSegments); segmentIndex++ {
		segment := evictedSegments[segmentIndex]
		if segment.State == changepointspb.SegmentState_FINALIZED {
			tvb.InsertFinalizedSegment(segment)
		} else { // Finalizing segment.
			tvb.FinalizingSegment = segment
			tvb.IsFinalizingSegmentDirty = true
		}
	}

	// Assert that finalizing segment is after finalized segments.
	tvb.verifyOutputBuffer()
}

func verifyEvictedSegments(evictedSegments []*changepointspb.Segment) {
	// Verify that evictedSegments contain all FINALIZED segment, except for
	// the last segment.
	for i, seg := range evictedSegments {
		if i != len(evictedSegments)-1 {
			if seg.State != changepointspb.SegmentState_FINALIZED {
				panic("evictedSegments should contains all finalized segments, except the last one")
			}
		} else {
			if seg.State != changepointspb.SegmentState_FINALIZED && seg.State != changepointspb.SegmentState_FINALIZING {
				panic("last segment of evicted segments should be finalizing or finalized")
			}
		}
	}
}

// verifyOutputBuffer verifies that the finalizing segment is older than any
// finalized segment.
// Panic if it is not the case.
func (tvb *TestVariantBranch) verifyOutputBuffer() {
	finalizedSegments := tvb.FinalizedSegments.GetSegments()
	l := len(finalizedSegments)
	if tvb.FinalizingSegment == nil || l == 0 {
		return
	}
	if finalizedSegments[l-1].EndPosition >= tvb.FinalizingSegment.StartPosition {
		panic("finalizing segment should be older than finalized segments")
	}
}

// combineSegment combine the finalizing segment from the output buffer with
// a segment evicted from the input buffer.
func combineSegment(finalizingSegment, evictedSegment *changepointspb.Segment) *changepointspb.Segment {
	result := &changepointspb.Segment{
		State:                        evictedSegment.State,
		HasStartChangepoint:          finalizingSegment.HasStartChangepoint,
		StartPosition:                finalizingSegment.StartPosition,
		StartHour:                    finalizingSegment.StartHour,
		StartPositionLowerBound_99Th: finalizingSegment.StartPositionLowerBound_99Th,
		StartPositionUpperBound_99Th: finalizingSegment.StartPositionUpperBound_99Th,
		EndPosition:                  evictedSegment.EndPosition,
		EndHour:                      evictedSegment.EndHour,
		FinalizedCounts:              addCounts(finalizingSegment.FinalizedCounts, evictedSegment.FinalizedCounts),
	}
	result.MostRecentUnexpectedResultHour = finalizingSegment.MostRecentUnexpectedResultHour
	if result.MostRecentUnexpectedResultHour.GetSeconds() < evictedSegment.MostRecentUnexpectedResultHour.GetSeconds() {
		result.MostRecentUnexpectedResultHour = evictedSegment.MostRecentUnexpectedResultHour
	}
	return result
}

// addCounts returns the sum of 2 statistics counts.
func addCounts(count1 *changepointspb.Counts, count2 *changepointspb.Counts) *changepointspb.Counts {
	return &changepointspb.Counts{
		TotalResults:             count1.TotalResults + count2.TotalResults,
		UnexpectedResults:        count1.UnexpectedResults + count2.UnexpectedResults,
		TotalRuns:                count1.TotalRuns + count2.TotalRuns,
		UnexpectedUnretriedRuns:  count1.UnexpectedUnretriedRuns + count2.UnexpectedUnretriedRuns,
		UnexpectedAfterRetryRuns: count1.UnexpectedAfterRetryRuns + count2.UnexpectedAfterRetryRuns,
		FlakyRuns:                count1.FlakyRuns + count2.FlakyRuns,
		TotalVerdicts:            count1.TotalVerdicts + count2.TotalVerdicts,
		UnexpectedVerdicts:       count1.UnexpectedVerdicts + count2.UnexpectedVerdicts,
		FlakyVerdicts:            count1.FlakyVerdicts + count2.FlakyVerdicts,
	}
}
