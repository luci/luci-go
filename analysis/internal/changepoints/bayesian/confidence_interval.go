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

package bayesian

import (
	"fmt"
	"math"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/model"
)

const (
	// Confidence interval tail is the default tail value for confidence analysis.
	// 0.005 tail gives 99% confidence interval.
	ConfidenceIntervalTail = 0.005
)

// ChangePointPositionDistribution returns a quantization of the
// changepoint commit start position probability distribution for
// the given history, assuming it has exactly one changepoint.
//
// history must be in increasing source position order.
//
// See PositionDistribution for interpretation.
//
// For theory background, please see go/bayesian-changepoint-estimation.
func (a ChangepointPredictor) ChangePointPositionDistribution(history []*inputbuffer.Run) *model.PositionDistribution {
	if len(history) == 0 {
		panic("test history is empty")
	}
	for i := 1; i < len(history); i++ {
		if history[i-1].SourcePosition > history[i].SourcePosition {
			panic("test history is not sorted in ascending source position order")
		}
	}

	// Stores the total for the entire history.
	var total counts
	for _, v := range history {
		total = total.addRun(v)
	}

	// Theory
	// ==============
	// The changepoint distribution is characterized by the following functions:
	//
	// Probability mass function (PMF):        P(C = k | Y)
	// Cumulative distribution function (CDF): P(C <= k | Y)
	//
	// where Y is the observations (i.e. test run results), C is the random variable
	// indicating the true changepoint source position, and k is a source position.
	//
	// In this method, we wish to compute the CDF, and we do so by integrating
	// PMF over values of k, i.e. P(C <= k | Y) = SUM over x <= k of P(C = x | Y).
	//
	// The PMF can be calculated as follows:
	// P (C = k | Y) = P(C = k AND Y) / P(Y)  { by Bayes theorem }
	//               = P(Y | C = k) * P(C = k) / P(Y)
	// Where:
	// - P(Y | C = k) is the likelihood of observing the given sequence of test
	//   runs Y assuming the changepoint is at position k. It is defined by multiplying
	//   the likelihood of the observing the sequence up to k-1 and the likelihood of
	//   observing the sequence from k, where the likelihood of observing a sequence
	//   is defined by our SequenceLikelihood library (bernoulli statistics).
	// - P(C = k) is the prior likelihood of observing a changepoint at source
	//   position k. As we do not have a model of how likely (apriori) a change
	//   is to cause a breakage, we assume a uniform distribution.
	//   - If ChangePointPredictor.SourcePositionsConsecutive is set, we assume
	//     k has a domain of (start_position, end_position], where start_position
	//     and end_position are the first and last positions in the given test history.
	//     For this case, P(C = k) = 1/D where D = (end_position-start_position).
	//   - If ChangePointPredictor.SourcePositionsConsecutive is unset, we can only
	//     assume the source positions mentioned in the `history` exist, i.e. k
	//     has a domain of unique_values(history.SourcePosition) except start_position,
	//     and therefore and P(C = k) = 1/D
	//     where D = (# of unique source positions in history except start_position).
	// - P(Y) is the likelihood of observing the given sequence of test runs Y.
	//   This can be computed from the above, as follows:
	//   P(Y) = SUM OVER k of P(C = k AND Y)
	//        = SUM OVER k of ((P(Y | C = k)*P(C = k))
	//        = SUM OVER k of ((P(Y | C = k)*(1/D))
	//   Note this method assumes exactly one changepoint exists (we do not consider
	//   the case where there is no changepoint).
	//
	// Practical considerations
	// ========
	// Test runs are often sparse, i.e. we have less than one test run
	// per commit position. For example, we may only have test runs for positions:
	// 1100, 1200, 1300, 1400, 1500, etc.
	//
	// In this case, it is wasteful to loop from position 1100 to 1500 computing
	// P(C = k | Y) for every point, particularly as computing sequence likelihoods
	// is moderately expensive. We can avoid these computations by observing that:
	// - P(C = k | Y) is the same for k = 1101 ... 1200
	// - P(C = k | Y) is the same for k = 1201 ... 1300
	//   and so on.
	// And therefore we only need to perform one computation per position.
	//
	// (Note: this only applies for ChangePointPredictor.SourcePositionsConsecutive,
	// otherwise we will assume the domain of k is limited to the source positions
	// we have test runs for.)
	//
	// Note that it is also possible we will have more than one test run
	// per position and in this case we will need to treat such test runs as one
	// unit as there cannot be a changepoint within a position.
	//
	// In many places we will use log-arithmetic to avoid running into
	// limits of floating point numbers, as some probabilities are very small.

	// Allocate some buffers to store sequence likelihoods conditional on
	// changepoint positions (P(Y | C = k)), and the start and end source
	// position ranges to which they apply.
	//
	// The following invariants will hold:
	// - for all i:
	//   - rangeLogLikelihoods[i]
	//      = log(SUM OVER k in (startPosition[i], endPosition[i]] OF P(Y | C = k))
	//
	// Note: if ChangePointPredictor.SourcePositionsConsecutive is false, the range
	// (startPosition[i], endPosition[i]] is taken to only contain endPosition[i] only
	// and thus the sum is over only one point.
	//
	// N.B. len(history) is strictly the maximum number of elements that will
	// be needed, we may use if there are multiple test runs for one source position.
	rangeLogLikelihoods := make([]float64, 0, len(history))
	startPositions := make([]int64, 0, len(history)) // Exclusive start of range
	endPositions := make([]int64, 0, len(history))   // Inclusive end of range

	// Create SequenceLikelihood to calculate the likelihood.
	firstTrySL := NewSequenceLikelihood(a.HasUnexpectedPrior)
	retrySL := NewSequenceLikelihood(a.UnexpectedAfterRetryPrior)

	// left counts the statistics to the left of a position.
	var left counts

	// Advance past the first commit position.
	// After this call, i will be at the first run of the next commit position.
	i, pending := nextPosition(history, 0)
	left = left.add(pending)

	// The start of the range of source positions represented by the next
	// loop iteration, exclusive.
	// The entire range is (startPosition, endPosition].
	startPosition := history[0].SourcePosition

	for i < len(history) {
		endPosition := history[i].SourcePosition

		// Invariants:
		// - `left` counts the statistics up to endPosition (exclusive).
		// - `right` counts the statistics from endPosition onwards (inclusive).
		// The word 'startPosition' could be substituted for 'endPosition' above
		// and the invariants would still hold.

		right := total.subtract(left)
		leftLikelihood := firstTrySL.LogLikelihood(left.HasUnexpected, left.Runs) + retrySL.LogLikelihood(left.UnexpectedAfterRetry, left.Retried)
		rightLikelihood := firstTrySL.LogLikelihood(right.HasUnexpected, right.Runs) + retrySL.LogLikelihood(right.UnexpectedAfterRetry, right.Retried)

		// conditionalLogLikelihood is log(P(Y | C = endPosition)).
		conditionalLogLikelihood := leftLikelihood + rightLikelihood

		// rangeLogLikelihood is  log(SUM OVER k in (startPosition[i], endPosition[i]] OF P(Y | C = k))
		var rangeLogLikelihood float64
		if a.SourcePositionsConsecutive {
			// As P(Y | C = k) is uniform for k in (startPosition,endPosition], compute
			// rangeLogLikelihood as: log(P(Y | C = endPosition) * num of positions).
			// Note also that log(a*b) = log(a) + log(b).
			rangeLogLikelihood = conditionalLogLikelihood + math.Log(float64(endPosition-startPosition))
		} else {
			// rangeLogLikelihood is P(Y | C = endPosition) * 1. As we do not infer
			// the existence of source positions without test results, there is only
			// one source position in the range (startPosition, endPosition], namely
			// endPosition.
			rangeLogLikelihood = conditionalLogLikelihood
		}

		startPositions = append(startPositions, startPosition)
		endPositions = append(endPositions, endPosition)
		rangeLogLikelihoods = append(rangeLogLikelihoods, rangeLogLikelihood)

		startPosition = endPosition

		// Advance to the next commit position.
		i, pending = nextPosition(history, i)
		left = left.add(pending)
	}

	// totalLogLikelihood calculates log(P(Y)) - log (1/D)
	// where D is the total number of candidate changepoint source positions
	// (see "Theory" above).
	//
	// P(Y) = SUM OVER k of P(C = k AND Y)
	//      = SUM OVER k of ((P(Y | C = k)*P(C = k))
	//      = SUM OVER k of ((P(Y | C = k)* 1/D)
	//      = 1/D * SUM OVER k of (P(Y | C = k))
	//
	// P(C = k) = 1/D because we assume a uniform distribution for
	// the prior of the changepoint position.
	// And so
	// log(P(Y)) = log(1/D * SUM OVER k of (P(Y | C = k))
	//           = log(1/D) + log(SUM OVER k of (P(Y | C = k)))
	//           = log(1/D) + totalLogLikelihood
	totalLogLikelihood := addLogLikelihoods(rangeLogLikelihoods)

	// Result will store a flattened/simplified version of the exact CDF we are
	// calculating here.
	var result model.PositionDistribution
	// resultIndex is the next slot in result we are looking to fill.
	resultIndex := 0
	// leftTailProbabilityTarget is the cumulative probability target, P(C <= k | Y),
	// we are aiming for to fill the next index in result.
	// Once we find it, we will store the value of the source position k that
	// caused the target to be exceeded.
	leftTailProbabilityTarget := model.TailLikelihoods[resultIndex]

	// Accumulates the likelihood of P(C <= endPosition[i] | Y).
	cumulativeLikelihood := 0.0

	for i := 0; i < len(rangeLogLikelihoods); i++ {
		// Calculate P(startPosition[i] < C <= endPosition[i] | Y).
		//  = SUM(for k in (startPosition[i], endPosition[i]] of P(C = k | Y))
		//
		// Reworking the inner expression:
		//    P(C = k | Y)
		//  = P(C = k AND Y) / P(Y)
		//  = P(Y | C = k) * P(C = k) / P(Y)
		//  = P(Y | C = k) * (1 / D) / P(Y)
		//
		// Inserting back into the main expression:
		//            SUM(for k in (startPosition[i], endPosition[i]] of P(C = k | Y))
		//  =         SUM(for k in (startPosition[i], endPosition[i]] of P(Y | C = k) * (1 / D) / P(Y) )            { substitute reworked innner expression }
		//  = exp(log(SUM(for k in (startPosition[i], endPosition[i]] of P(Y | C = k) * (1 / D) / P(Y) )         )) { using exp(log(x)) = x, where x != 0 }
		//  = exp(log(SUM(for k in (startPosition[i], endPosition[i]] of P(Y | C = k)) + log((1 / D)) - log(P(Y) )  { by log laws }
		//  = exp(rangeLogLikelihoods[i]                                               + log((1 / D)) - log(P(Y) )  { substitute rangeLogLikelihoods[i] }
		//  = exp(rangeLogLikelihoods[i]                                               - totalLogLikelihood )       { substitute totalLogLikelihood }
		rangeLikelihood := math.Exp(rangeLogLikelihoods[i] - totalLogLikelihood)

		// While P(C <= endPosition[i] | Y) will exceed the next left tail probability target
		// we wish to sample.
		for (cumulativeLikelihood+rangeLikelihood) >= leftTailProbabilityTarget && resultIndex < len(result) {
			// The accumulation until the previous end position, endPosition[i-1],
			// did not exceed the cumulative probability target, but once we include
			// the range up to this end position it does exceed.
			// I.E. P(C <= endPosition[i-1] | Y) < leftTailProbabilityTarget but
			//      P(C <= endPosition[i] | Y) >= leftTailProbabilityTarget

			var k int64

			if leftTailProbabilityTarget < 0.5 {
				// As we are in the left tail, pick the start position to be more conservative.
				// On the UI, we should only really highlight startPositions[i] + 1 onwards
				// as that the position where the risk of changepoint has started to go up but we
				// want to always anchor to real positions for now.
				k = startPositions[i]
			} else if leftTailProbabilityTarget == 0.5 {
				// Center of the distribution.
				// As we want to always anchor to a position with at test result, we could snap
				// to either startPositions[i] or endPositions[i]. We prefer to pick the end
				// position as the jump in cumulativeLikelihood is attributable to it.
				k = endPositions[i]
			} else { // leftTailProbabilityTarget > 0.5
				// As we in the right tail, pick the end position to be more conservative.
				// On the UI, we want to highlight the confidence interval as being inclusive of
				// this position as an elevated risk of a changepoint occurring extends to this
				// position.
				k = endPositions[i]
			}

			result[resultIndex] = k

			resultIndex++
			if resultIndex < len(result) {
				// Find the cumulative probability target
				leftTailProbabilityTarget = model.TailLikelihoods[resultIndex]
			}
		}

		// Accumulate P(C <= endPosition[i] | Y).
		cumulativeLikelihood += rangeLikelihood

		// Advance to the next position.
	}

	if resultIndex < len(result) {
		// This should not happen. So long as cumulativeLikelihood exceeds
		// model.LeftTailProbability[model.TailLikelihoodsLength-1], all positions in
		// result should be filled in. We actually expect cumulativeLikelihood = 1.0
		// at this point barring any floating point imprecisions.
		// It is not believed floating point precisions can exceed the
		// maximum this method can tolerate (as at writing 1.0-0.9995=0.0005).
		panic(fmt.Errorf("result not filled in, final cumulative likelihood = %v",
			cumulativeLikelihood))
	}

	return &result
}

// ChangePoints runs changepoint detection and confidence
// interval analysis for history.
// history is sorted by commit position ascendingly (oldest commit first).
func (a ChangepointPredictor) ChangePoints(history []*inputbuffer.Run, tail float64) []inputbuffer.ChangePoint {
	changePointIndices := a.identifyChangePoints(history)

	// For simplicity, we add a fake index to the end.
	ids := make([]int, len(changePointIndices)+1)
	copy(ids, changePointIndices)
	ids[len(ids)-1] = len(history)

	result := []inputbuffer.ChangePoint{}
	startIndex := 0
	for i := 1; i < len(ids); i++ {
		runs := history[startIndex:ids[i]]
		dist := a.ChangePointPositionDistribution(runs)
		result = append(result, inputbuffer.ChangePoint{
			NominalIndex:         ids[i-1],
			PositionDistribution: dist,
		})
		startIndex = ids[i-1]
	}
	return result
}
