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

// Package analyzer converts the input buffer into segments using
// changepoint analysis, and synthesises those segments with the
// segments in the output buffer to produce a logical segementation
// of the test history.
package analyzer

import (
	"time"

	"go.chromium.org/luci/analysis/internal/changepoints/bayesian"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
)

type Analyzer struct {
	// MergeBuffer is a preallocated buffer used to store the result of
	// merging hot and cold input buffers. Reusing the same buffer avoids
	// allocating a new buffer for each test variant branch processed.
	// Sorted by commit position (oldest first), then result hour (oldest first).
	mergeBuffer []inputbuffer.Run
}

// Run runs changepoint analysis, performs any required
// evictions (from the hot input buffer to the cold input buffer,
// and from the input buffer to the output buffer),
// and returns the logical segments for the test variant branch.
//
// Segments are sorted by commit position (most recent first).
func (a *Analyzer) Run(tvb *testvariantbranch.Entry) []Segment {
	predictor := bayesian.ChangepointPredictor{
		ChangepointLikelihood: 0.0001,
		// We are leaning toward consistently passing test results.
		HasUnexpectedPrior: bayesian.BetaDistribution{
			Alpha: 0.3,
			Beta:  0.5,
		},
		UnexpectedAfterRetryPrior: bayesian.BetaDistribution{
			Alpha: 0.5,
			Beta:  0.5,
		},
	}
	tvb.InputBuffer.CompactIfRequired()
	tvb.InputBuffer.MergeBuffer(&a.mergeBuffer)
	changePoints := predictor.ChangePoints(a.mergeBuffer, bayesian.ConfidenceIntervalTail)
	sib := tvb.InputBuffer.Segmentize(a.mergeBuffer, changePoints)
	evictedSegments := sib.EvictSegments()
	if len(evictedSegments) > 0 {
		tvb.UpdateOutputBuffer(evictedSegments)
		// Update merge buffer to reflect new state of the input buffer.
		tvb.InputBuffer.MergeBuffer(&a.mergeBuffer)
	}
	return toSegments(tvb, sib.Segments, a.mergeBuffer)
}

// toSegments returns the segments for a test variant branch, using
// the provided input buffer segments and merged runs for the input buffer
// portion.
// The segments returned will be sorted, with the most recent segment
// comes first.
func toSegments(tvb *testvariantbranch.Entry, inputBufferSegments []*inputbuffer.Segment, mergedRuns []inputbuffer.Run) []Segment {
	results := []Segment{}

	// The index where the active segments starts.
	// If there is a finalizing segment, then the we need to first combine it will
	// the first segment from the input buffer.
	activeStartIndex := 0
	if tvb.FinalizingSegment != nil {
		activeStartIndex = 1
	}

	// Add the active segments.
	for i := len(inputBufferSegments) - 1; i >= activeStartIndex; i-- {
		inputSegment := inputBufferSegments[i]
		segment := inputSegmentToSegment(inputSegment, mergedRuns[inputSegment.StartIndex:inputSegment.EndIndex+1])
		results = append(results, segment)
	}

	// Add the finalizing segment.
	if tvb.FinalizingSegment != nil {
		inputSegment := inputBufferSegments[0]
		bqSegment := combineSegment(tvb.FinalizingSegment, inputSegment, mergedRuns[inputSegment.StartIndex:inputSegment.EndIndex+1])
		results = append(results, bqSegment)
	}

	// Add the finalized segments.
	if tvb.FinalizedSegments != nil {
		// More recent segments are on the back.
		for i := len(tvb.FinalizedSegments.Segments) - 1; i >= 0; i-- {
			outputSegment := tvb.FinalizedSegments.Segments[i]
			segment := finalizedSegmentToSegment(outputSegment)
			results = append(results, segment)
		}
	}

	return results
}

// inputSegmentToSegment constructs a logical segment from an input buffer segment.
func inputSegmentToSegment(inputSegment *inputbuffer.Segment, runs []inputbuffer.Run) Segment {
	return Segment{
		HasStartChangepoint:            inputSegment.HasStartChangepoint,
		StartPosition:                  inputSegment.StartPosition,
		StartPositionLowerBound99Th:    inputSegment.StartPositionLowerBound99Th,
		StartPositionUpperBound99Th:    inputSegment.StartPositionUpperBound99Th,
		StartHour:                      inputSegment.StartHour,
		EndPosition:                    inputSegment.EndPosition,
		EndHour:                        inputSegment.EndHour,
		MostRecentUnexpectedResultHour: inputSegment.MostRecentUnexpectedResultHour,
		Counts:                         segmentCounts(nil, runs),
	}
}

// combineSegment constructs a logical segment from its finalizing part in
// the output buffer and its unfinalized part in the input buffer.
func combineSegment(finalizingSegment *cpb.Segment, inputSegment *inputbuffer.Segment, inputSegmentRuns []inputbuffer.Run) Segment {
	var mostRecentUnexpectedResultHour time.Time
	if finalizingSegment.MostRecentUnexpectedResultHour != nil {
		mostRecentUnexpectedResultHour = finalizingSegment.MostRecentUnexpectedResultHour.AsTime()
	}
	if inputSegment.MostRecentUnexpectedResultHour.After(mostRecentUnexpectedResultHour) {
		mostRecentUnexpectedResultHour = inputSegment.MostRecentUnexpectedResultHour
	}

	return Segment{
		HasStartChangepoint:            finalizingSegment.HasStartChangepoint,
		StartPosition:                  finalizingSegment.StartPosition,
		StartHour:                      finalizingSegment.StartHour.AsTime(),
		StartPositionLowerBound99Th:    finalizingSegment.StartPositionLowerBound_99Th,
		StartPositionUpperBound99Th:    finalizingSegment.StartPositionUpperBound_99Th,
		EndPosition:                    inputSegment.EndPosition,
		EndHour:                        inputSegment.EndHour,
		MostRecentUnexpectedResultHour: mostRecentUnexpectedResultHour,
		Counts:                         segmentCounts(finalizingSegment.FinalizedCounts, inputSegmentRuns),
	}
}

// finalizedSegmentToSegment constructs a logical segment from a finalized segment in
// the output buffer.
func finalizedSegmentToSegment(finalizedSegment *cpb.Segment) Segment {
	var mostRecentUnexpectedResultHour time.Time
	if finalizedSegment.MostRecentUnexpectedResultHour != nil {
		mostRecentUnexpectedResultHour = finalizedSegment.MostRecentUnexpectedResultHour.AsTime()
	}

	return Segment{
		HasStartChangepoint:            finalizedSegment.HasStartChangepoint,
		StartPosition:                  finalizedSegment.StartPosition,
		StartPositionLowerBound99Th:    finalizedSegment.StartPositionLowerBound_99Th,
		StartPositionUpperBound99Th:    finalizedSegment.StartPositionUpperBound_99Th,
		StartHour:                      finalizedSegment.StartHour.AsTime(),
		EndPosition:                    finalizedSegment.EndPosition,
		EndHour:                        finalizedSegment.EndHour.AsTime(),
		MostRecentUnexpectedResultHour: mostRecentUnexpectedResultHour,
		Counts:                         segmentCounts(finalizedSegment.FinalizedCounts, nil),
	}
}

// segmentCounts computes counts for logical segment by combining
// stored segment counts from the output buffer (if any) with
// counts for runs still in the input buffer.
func segmentCounts(counts *cpb.Counts, runs []inputbuffer.Run) Counts {
	var partialSourceVerdict *cpb.PartialSourceVerdict
	var result Counts
	if counts != nil {
		// Start with the counts from the output buffer.
		//
		// For source verdict counts, this is stored as two parts:
		// - a final part and
		// - a pending part for the last source verdict seen by the
		//   output buffer, which may only have been partially
		//   evicted from the input buffer.
		partialSourceVerdict = counts.PartialSourceVerdict
		result = Counts{
			// Test results.
			UnexpectedResults:        counts.UnexpectedResults,
			TotalResults:             counts.TotalResults,
			ExpectedPassedResults:    counts.ExpectedPassedResults,
			ExpectedFailedResults:    counts.ExpectedFailedResults,
			ExpectedCrashedResults:   counts.ExpectedCrashedResults,
			ExpectedAbortedResults:   counts.ExpectedAbortedResults,
			UnexpectedPassedResults:  counts.UnexpectedPassedResults,
			UnexpectedFailedResults:  counts.UnexpectedFailedResults,
			UnexpectedCrashedResults: counts.UnexpectedCrashedResults,
			UnexpectedAbortedResults: counts.UnexpectedAbortedResults,
			// Runs.
			UnexpectedUnretriedRuns:  counts.UnexpectedUnretriedRuns,
			UnexpectedAfterRetryRuns: counts.UnexpectedAfterRetryRuns,
			FlakyRuns:                counts.FlakyRuns,
			TotalRuns:                counts.TotalRuns,
			// Source Verdicts.
			FlakySourceVerdicts:      counts.FlakySourceVerdicts,
			UnexpectedSourceVerdicts: counts.UnexpectedSourceVerdicts,
			TotalSourceVerdicts:      counts.TotalSourceVerdicts,
		}
	}

	verdictStream := testvariantbranch.NewRunStreamAggregator(partialSourceVerdict)
	for _, run := range runs {

		// Result-level statistics.
		result.TotalResults += int64(run.Expected.Count() + run.Unexpected.Count())
		result.UnexpectedResults += int64(run.Unexpected.Count())
		result.ExpectedPassedResults += int64(run.Expected.PassCount)
		result.ExpectedFailedResults += int64(run.Expected.FailCount)
		result.ExpectedCrashedResults += int64(run.Expected.CrashCount)
		result.ExpectedAbortedResults += int64(run.Expected.AbortCount)
		result.UnexpectedPassedResults += int64(run.Unexpected.PassCount)
		result.UnexpectedFailedResults += int64(run.Unexpected.FailCount)
		result.UnexpectedCrashedResults += int64(run.Unexpected.CrashCount)
		result.UnexpectedAbortedResults += int64(run.Unexpected.AbortCount)

		// Run-level statistics,
		result.TotalRuns++
		// flaky run.
		isFlakyRun := run.Expected.Count() > 0 && run.Unexpected.Count() > 0
		if isFlakyRun {
			result.FlakyRuns++
		}
		// unexpected unretried run.
		isUnexpectedUnretried := run.Unexpected.Count() == 1 && run.Expected.Count() == 0
		if isUnexpectedUnretried {
			result.UnexpectedUnretriedRuns++
		}
		// unexpected after retries run.
		isUnexpectedAfterRetries := run.Unexpected.Count() > 1 && run.Expected.Count() == 0
		if isUnexpectedAfterRetries {
			result.UnexpectedAfterRetryRuns++
		}

		v, ok := verdictStream.Insert(run)
		if !ok {
			// No verdict yielded.
			continue
		}
		// Add verdict to hourly bucket.
		result.TotalSourceVerdicts++

		if v.UnexpectedResults > 0 {
			if v.ExpectedResults > 0 {
				result.FlakySourceVerdicts++
			} else {
				result.UnexpectedSourceVerdicts++
			}
		}
	}

	// Merge the pending verdict into the count, so that we
	// can give the count at this point in time.
	lastVerdict := verdictStream.SaveState()
	if lastVerdict != nil {
		result.TotalSourceVerdicts++
		if lastVerdict.UnexpectedResults > 0 {
			if lastVerdict.ExpectedResults > 0 {
				result.FlakySourceVerdicts++
			} else {
				result.UnexpectedSourceVerdicts++
			}
		}
	}
	return result
}
