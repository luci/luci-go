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
	"math"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
)

const (
	// Confidence interval tail is the default tail value for confidence analysis.
	// 0.005 tail gives 99% confidence interval.
	ConfidenceIntervalTail = 0.005
)

// changepointPositionConfidenceInterval returns the (100% - (2 * tail))
// two-tailed confidence interval for a changepoint that occurs in the
// given slice of history.
// E.g. tail = 0.005 gives the 99% confidence interval.
// For details, please see go/bayesian-changepoint-estimation.
// In practice, after detecting a changepoint, we will run this function for
// the combination of the left and the right segments of the changepoints.
// For example, if there are 3 segments:
// [3, 9], [10, 15], [16, 21].
// To analyze the confidence interval around change point at position 16, we
// will run this function with the slice of history between 10 and 21.
func (a ChangepointPredictor) changepointPositionConfidenceInterval(history []inputbuffer.PositionVerdict, tail float64) (min int, max int) {
	length := len(history)
	if length == 0 {
		panic("test history is empty")
	}

	// Stores the total for the entire history.
	var total counts
	for _, v := range history {
		total = total.addVerdict(v)
	}

	// changepointLogLikelihoods stores the log-likelihood of the observing the
	// given test verdict sequence.
	// i.e. log(P(Y | C = k)),
	// where Y is the observations, and "C = k" means there is a changepoint at
	// position k.
	// We only store the loglikelihood of positions that mark the start of a new
	// commit, positions that are not the start of a new commit will be skipped.
	// Store changepointIndices so we can map back to the original slice later.
	changepointIndices := []int{}
	changepointLogLikelihoods := []float64{}

	// Create SequenceLikelihood to calculate the likelihood.
	firstTrySL := NewSequenceLikelihood(a.HasUnexpectedPrior)
	retrySL := NewSequenceLikelihood(a.UnexpectedAfterRetryPrior)

	// left counts the statistics to the left of a position.
	var left counts

	// Advance past the first commit position.
	i, pending := nextPosition(history, 0)
	left = left.add(pending)

	for i < length {
		// right counts the statistics from [i:].
		right := total.subtract(left)
		leftLikelihood := firstTrySL.LogLikelihood(left.HasUnexpected, left.Runs) + retrySL.LogLikelihood(left.UnexpectedAfterRetry, left.Retried)
		rightLikelihood := firstTrySL.LogLikelihood(right.HasUnexpected, right.Runs) + retrySL.LogLikelihood(right.UnexpectedAfterRetry, right.Retried)

		// conditionalLikelihood is log(P(Y | C = i)).
		conditionalLikelihood := leftLikelihood + rightLikelihood
		changepointIndices = append(changepointIndices, i)
		changepointLogLikelihoods = append(changepointLogLikelihoods, conditionalLikelihood)

		// Advance to the next commit position.
		nextIndex, pending := nextPosition(history, i)
		left = left.add(pending)
		i = nextIndex
	}

	// totalLogLikelihood calculates log(P(Y)) - log (1/D).
	// where D is the length of changepointLogLikelihoods.
	// P(Y) = SUM OVER k of P(C = k AND Y)
	//      = SUM OVER k of ((P(Y | C = k)*P(C = k))
	//      = SUM OVER k of ((P(Y | C = k)* 1/D)
	//      = 1/D * SUM OVER k of (P(Y | C = k)
	// because we assume a uniform distribution for P(C = k).
	// So
	// log(P(Y)) = log(1/D * SUM OVER k of (P(Y | C = k))
	//           = log(1/D) + log(SUM OVER k of (P(Y | C = k)))
	//           = log(1/D) + totalLogLikelihood
	totalLogLikelihood := AddLogLikelihoods(changepointLogLikelihoods)

	// Stores the likelihood of P(C <= t | Y).
	cumulativeLikelihood := 0.0
	min = 0
	max = len(changepointLogLikelihoods) - 1

	for k := 0; k < len(changepointLogLikelihoods); k++ {
		// Calculates P(C = k | Y).
		// P (C = k | Y) = P(C = k AND Y) / P(Y)
		//               = P(Y | C = k) * P(C = k) / P(Y)
		//               = P(Y | C = k) * (1 / D) / P(Y)
		//               = exp (log (P(Y | C = k) * (1 / D) / P(Y)))
		//               = exp (log(P(Y | C = k) + log (1/D) - log (P(Y))
		//               = exp (changepointLogLikelihoods[k] + log (1/D) - log(P(Y))
		//               = exp (changepointLogLikelihoods[k] + log (1/D) - (log(1/D) + totalLogLikelihood))
		//               = exp (changepointLogLikelihoods[k] - totalLogLikelihood)
		changepointLikelihood := math.Exp(changepointLogLikelihoods[k] - totalLogLikelihood)
		cumulativeLikelihood += changepointLikelihood

		if cumulativeLikelihood < tail {
			min = k
		}
		if cumulativeLikelihood > (1.0-tail) && max == (len(changepointLogLikelihoods)-1) {
			max = k
		}
	}

	return changepointIndices[min], changepointIndices[max]
}

// ChangePoints runs change point detection and confidence
// interval analysis for history.
// history is sorted by commit position ascendingly (oldest commit first).
func (a ChangepointPredictor) ChangePoints(history []inputbuffer.PositionVerdict, tail float64) []inputbuffer.ChangePoint {
	changepointIndices := a.identifyChangePoints(history)

	// For simplicity, we add a fake index to the end.
	ids := make([]int, len(changepointIndices)+1)
	copy(ids, changepointIndices)
	ids[len(ids)-1] = len(history)

	result := []inputbuffer.ChangePoint{}
	startIndex := 0
	for i := 1; i < len(ids); i++ {
		vds := history[startIndex:ids[i]]
		// min and max needs to be offset by startIndex.
		min, max := a.changepointPositionConfidenceInterval(vds, ConfidenceIntervalTail)
		result = append(result, inputbuffer.ChangePoint{
			NominalIndex:        ids[i-1],
			LowerBound99ThIndex: min + startIndex,
			UpperBound99ThIndex: max + startIndex,
		})
		startIndex = ids[i-1]
	}
	return result
}
