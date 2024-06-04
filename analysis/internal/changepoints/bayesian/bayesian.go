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

// Package bayesian implements bayesian analysis for detecting change points.
package bayesian

import (
	"math"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
)

type ChangepointPredictor struct {
	// Threshold for creating new change points.
	ChangepointLikelihood float64

	// The prior for the rate at which a test's runs have any
	// unexpected test result.
	// This is the prior for estimating the ratio
	// HasUnexpected / Runs of a segment.
	//
	// Generally tests tend to be either consistently passing or
	// consistently failing, with a bias towards consistently
	// passing, so shape parameters Alpha < 1, Beta < 1, Alpha < Beta
	// are typically selected (e.g. alpha = 0.3, beta = 0.5).
	HasUnexpectedPrior BetaDistribution

	// The prior for the rate at which a test's runs have
	// only unexpected results, given they have at least
	// two results and one is unexpected.
	//
	// This is the prior for estimating UnexpectedAfterRetry / Retried.
	// Generally the result of retrying a fail inside a test run
	// either leads to a pass (fairly consistently) or another failure
	// (fairly consistently). Consequently, shape parameters Alpha < 1,
	// Beta < 1 are advised (e.g. alpha = 0.5, beta = 0.5).
	UnexpectedAfterRetryPrior BetaDistribution
}

// identifyChangePoints identifies all change point for given test history.
//
// This method requires the provided history to be sorted by commit position
// (either ascending or descending is fine).
// It allows multiple runs to be specified per commit position, by
// including those runs as adjacent elements in the history slice.
//
// This function returns the indices (in the history slice) of the change points
// identified. If an index i is returned, it means the history is segmented as
// history[:i] and history[i:].
// The indices returned are sorted ascendingly (lowest index first).
func (a ChangepointPredictor) identifyChangePoints(history []*inputbuffer.Run) []int {
	if len(history) == 0 {
		panic("test history is empty")
	}

	relativeLikelihood, bestChangepoint := a.FindBestChangepoint(history)
	if (relativeLikelihood + math.Log(a.ChangepointLikelihood)) <= 0 {
		// Do not split.
		return nil
	}
	// Identify further split points on the left and right hand sides, recursively.
	result := a.identifyChangePoints(history[:bestChangepoint])
	result = append(result, bestChangepoint)
	rightChangepoints := a.identifyChangePoints(history[bestChangepoint:])
	for _, changePoint := range rightChangepoints {
		// Adjust the offset of splitpoints in the right half,
		// from being relative to the start of the right half
		// to being relative to the start of the entire history.
		result = append(result, changePoint+bestChangepoint)
	}
	return result
}

// FindBestChangepoint finds the change point position that maximises
// the likelihood of observing the given test history.
//
// It returns the position of the change point in the history slice,
// as well as the change in log-likelihood attributable to the change point,
// relative to the `no change point` case.
//
// The semantics of the returned position are as follows:
// a position p means the history is segmented as
// history[:p] and history[p:].
// If the returned position is 0, it means no change point position was
// better than the `no change point` case.
//
// This method requires the provided history to be sorted by
// commit position (either ascending or descending is fine).
// It allows multiple runs to be specified per
// commit position, by including those runs as adjacent
// elements in the history slice.
//
// Note that if multiple runs are specified per commit position,
// the returned position will only ever be between two commit
// positions in the history, i.e. it holds that
// history[position-1].CommitPosition != history[position].CommitPosition
// (or position == 0).
//
// This method assumes a uniform prior for all change point positions,
// including the no change point case.
// If we are to bias towards the no change point case, thresholding
// should be applied to relativeLikelihood before considering the
// change point real.
func (a ChangepointPredictor) FindBestChangepoint(history []*inputbuffer.Run) (relativeLikelihood float64, position int) {
	length := len(history)

	// Stores the total for the entire history.
	var total counts
	for _, v := range history {
		total = total.addRun(v)
	}

	// Calculate the absolute log-likelihood of observing the
	// history assuming there is no change point.
	firstTrySL := NewSequenceLikelihood(a.HasUnexpectedPrior)
	retrySL := NewSequenceLikelihood(a.UnexpectedAfterRetryPrior)
	prioriLogLikelihood := firstTrySL.LogLikelihood(total.HasUnexpected, total.Runs) + retrySL.LogLikelihood(total.UnexpectedAfterRetry, total.Retried)

	// bestChangepoint represents the index of the best change point.
	// The change point is said to occur before the corresponding slice
	// element, so that results[:bestChangepoint] and results[bestChangepoint:]
	// represents the two distinct test history series divided by the
	// change point.
	bestChangepoint := 0
	bestLikelihood := -math.MaxFloat64

	// leftUnexpected stores the totals for result positions
	// history[0...i-1 (inclusive)].
	var i int
	var left counts

	// A heuristic for determining which points in the history
	// are interesting to evaluate.
	var heuristic changePointHeuristic

	// The provided history may have multiple runs for the same
	// commit position. As we should only consider change points between
	// commit positions (not inside them), we will iterate over the
	// history using nextPosition().

	// Advance past the first commit position.
	i, pending := nextPosition(history, 0)
	left = left.add(pending)
	heuristic.addToHistory(pending)

	for i < length {
		// Find the end of the next commit position.
		// Pending contains the counts from history[i:nextIndex].
		nextIndex, pending := nextPosition(history, i)

		// Only consider change points at positions that
		// are heuristically likely, to save on compute cycles.
		// The heuristic is designed to be consistent with
		// the sequence likelihood model, so will not eliminate
		// evaluation of positions that have no chance of
		// maximising bestLikelihood.
		if heuristic.isChangepointPossibleWithNext(pending) {
			right := total.subtract(left)

			// Calculate the likelihood of observing sequence
			// given there is a change point at this position.
			leftLikelihood := firstTrySL.LogLikelihood(left.HasUnexpected, left.Runs) + retrySL.LogLikelihood(left.UnexpectedAfterRetry, left.Retried)
			rightLikelihood := firstTrySL.LogLikelihood(right.HasUnexpected, right.Runs) + retrySL.LogLikelihood(right.UnexpectedAfterRetry, right.Retried)
			conditionalLikelihood := leftLikelihood + rightLikelihood
			if conditionalLikelihood > bestLikelihood {
				bestChangepoint = i
				bestLikelihood = conditionalLikelihood
			}
		}

		// Advance to the next commit position.
		left = left.add(pending)
		heuristic.addToHistory(pending)
		i = nextIndex
	}
	return bestLikelihood - prioriLogLikelihood, bestChangepoint
}

// nextPosition allows iterating over test history one commit position at a time.
//
// It finds the index `nextIndex` that represents advancing exactly one commit
// position from `index`, and returns the counts of runs that were
// advanced over.
//
// If there is only one run for a commit position, nextIndex will be index + 1,
// otherwise, if there are a number of runs for a commit position, nextIndex
// will be advanced by that number.
//
// Preconditions:
// The provided history is in order by commit position (either ascending or
// descending order is fine).
func nextPosition(history []*inputbuffer.Run, index int) (nextIndex int, pending counts) {
	// The commit position for which we are accumulating test runs.
	commitPosition := history[index].CommitPosition

	var c counts
	nextIndex = index
	for ; nextIndex < len(history) && history[nextIndex].CommitPosition == commitPosition; nextIndex++ {
		c = c.addRun(history[nextIndex])
	}
	return nextIndex, c
}
