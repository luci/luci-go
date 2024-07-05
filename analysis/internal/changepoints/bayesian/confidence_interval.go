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
// See PositionDistribution for interpretation.
//
// For theory background, please see go/bayesian-changepoint-estimation.
func (a ChangepointPredictor) ChangePointPositionDistribution(history []*inputbuffer.Run) *model.PositionDistribution {
	if len(history) == 0 {
		panic("test history is empty")
	}
	for i := 1; i < len(history); i++ {
		if history[i-1].CommitPosition > history[i].CommitPosition {
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
	// where Y is the observations (i.e. test runs), C is the random variable
	// indicating the true changepoint source position, and k is a source position.
	//
	// In this method, we treat source positions like k as having a domain of
	// start_position (exclusive) to end_position (inclusive) where start_position
	// and end_position are the first and last positions in the given test history.
	//
	// In this method, we wish to compute the CDF, and we do so by integrating
	// PMF over values of k, i.e. P(C <= k | Y) = SUM over x <= k of P(C = x | Y).
	//
	// The PMF can be calculated as follows:
	// P (C = k | Y) = P(C = k AND Y) / P(Y)
	//               = P(Y | C = k) * P(C = k) / P(Y)
	// Where:
	// - P(Y | C = k) is the likelihood of observing the given sequence of test
	//   runs Y assuming the changepoint is at position k. It is defined by multiplying
	//   the likelihood of the observing the sequence up to k-1 and the likelihood of
	//   observing the sequence from k, where the likelihood of observing a sequence
	//   is defined by SequenceLikelihood (bernoulli statistics).
	// - P(C = k) is the prior likelihood of observing a changepoint at source
	//   position k. As we do not have a model of how likely (apriori) a change
	//   is to cause a breakage, we assume a uniform distribution.
	//   Given k has a domain of size D, where D = end_position-start_position,
	//   P(C = k) = 1/D. Note end_position is inclusive and start_position is exclusive.
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
	//   - for k in startPositions[i] to endPositions[i]:
	//       conditionalLogLikelihoods[i] = log(P(Y | C = k))
	//     AND
	//   - rangeLogLikelihoods[i] =
	//       log(SUM OVER k from startPosition[i] to endPosition[i] OF P(Y | C = k))
	//
	// N.B. len(history) is strictly the maximum number of elements that will
	// be needed, we may use if there are multiple test runs for one source position.
	conditionalLogLikelihoods := make([]float64, 0, len(history))
	rangeLogLikelihoods := make([]float64, 0, len(history))
	startPositions := make([]int64, 0, len(history))
	endPositions := make([]int64, 0, len(history))

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
	// loop iteration.
	startPosition := history[0].CommitPosition + 1

	for i < len(history) {
		endPosition := history[i].CommitPosition

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
		// rangeLogLikelihood is log(P(Y | C = endPosition) * num of positions).
		// Note: log(a*b) = log(a) + log(b).
		rangeLogLikelihood := conditionalLogLikelihood + math.Log(float64(endPosition-startPosition+1))

		startPositions = append(startPositions, startPosition)
		endPositions = append(endPositions, endPosition)
		conditionalLogLikelihoods = append(conditionalLogLikelihoods, conditionalLogLikelihood)
		rangeLogLikelihoods = append(rangeLogLikelihoods, rangeLogLikelihood)

		startPosition = endPosition + 1

		// Advance to the next commit position.
		i, pending = nextPosition(history, i)
		left = left.add(pending)
	}

	// totalLogLikelihood calculates log(P(Y)) - log (1/D)
	// where D is the total number of candidate changepoint source positions
	// (i.e. history[len(history)-1].CommitPosition - history[0].CommitPosition).
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

	for i := 0; i < len(conditionalLogLikelihoods); i++ {
		// Calculates log(P(C = k | Y)), for k being any in startPosition[i] (inclusive)
		// to endPosition[i] (inclusive).
		//
		//    log(P (C = k | Y))
		//  = log (P(C = k AND Y) / P(Y))
		//  = log (P(Y | C = k) * P(C = k) / P(Y))
		//  = log (P(Y | C = k) * (1 / D) / P(Y))
		//  = log (P(Y | C = k) + log (1/D) - log (P(Y))
		//  = conditionalLogLikelihoods[k] + log (1/D) - log(P(Y))
		//  = conditionalLogLikelihoods[k] + log (1/D) - (log(1/D) + totalLogLikelihood)
		//  = conditionalLogLikelihoods[k] - totalLogLikelihood
		positionLogLikelihood := conditionalLogLikelihoods[i] - totalLogLikelihood

		// Calculate P(startPosition[i] <= C <= endPosition[i] | Y).
		// Invariant: rangeLikelihood = math.Exp(positionLogLikelihood) * (endPosition[i] - startPosition[i] + 1)
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
				// Snap to the start of the range instead of finding the exact position.
				// Note that the bounds of confidence interval are inclusive not exclusive,
				// so we do not need to subtract one.
				k = startPositions[i]
			} else if leftTailProbabilityTarget == 0.5 {
				// Center of the distribution.
				// Find the exact position in that range from startPosition[i] to endPosition[i]
				// first causes the target to be exceeded.

				// Example:
				// leftTailProbabilityTarget = 0.10
				// cumulativeLikelihood = 0.065
				// position likelihood = 0.01  (positionLogLikelihood of -4.605)
				//
				// startPosition[i]   increases cumulativeLikelihood from 0.065 to 0.075
				// startPosition[i]+1 increases cumulativeLikelihood from 0.075 to 0.085
				// startPosition[i]+2 increases cumulativeLikelihood from 0.085 to 0.095
				// startPosition[i]+3 increases cumulativeLikelihood from 0.095 to 0.105
				//
				// Below we compute (0.10 - 0.065) / 0.01 = 3.5 positions from the start.
				//
				// Excluding log arithmetic, we are calculating:
				// offsetFP = math.Floor((leftTailProbabilityTarget - cumulativeLikelihood) / positionLikelihood)
				//
				// We use that: A / B = exp(log(A/B)) = exp(log(A) - log(B)).
				//
				// If leftTailProbabilityTarget-cumulativeLikelihood is zero, then math.Log(0) = -Inf,
				// and math.Exp(-Inf) = 0, so we still get the expected result.
				offsetFP := math.Floor(math.Exp(math.Log(leftTailProbabilityTarget-cumulativeLikelihood) - positionLogLikelihood))
				if math.IsInf(offsetFP, 0) || math.IsNaN(offsetFP) {
					// In rare cases the computation above may not have enough precision
					// to return a sensible result. In this case just pick one of the
					// positions in startPositions[i] to endPositions[i].
					k = endPositions[i]
				} else {
					k = startPositions[i] + int64(offsetFP)
					// Check we are still in the correct range, floating point
					// imprecisions could mean we are off.
					if k < startPositions[i] {
						k = startPositions[i]
					} else if k > endPositions[i] {
						k = endPositions[i]
					}
				}
			} else { // leftTailProbabilityTarget > 0.5
				// Snap to the end of the range instead of finding the exact position.
				// Note that the bounds of confidence interval are inclusive not exclusive,
				// so we do not need to add one.
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
