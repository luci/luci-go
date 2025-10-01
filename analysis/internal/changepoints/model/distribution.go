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

// Package model contains common types used in changepoint analysis.
package model

import (
	"fmt"

	"golang.org/x/exp/slices"
)

// TailLikelihoodsLength is the number of points the changepoint
// position probability distribution is reduced to by PositionDistribution.
const TailLikelihoodsLength = 17

// TailLikelihoods defines the points on the changepoint position
// cumulative distribution function that PositionDistribution will store.
//
// I.E. the values of P(C <= k | Y) for which to record k, where:
//   - C is the random variable representing the (unknown) true changepoint
//     position
//   - k is the upper bound on the changepoint position.
//   - Y is the test history
//
// We do not wish to store the exact changepoint position distribution
// in its full fidelity as that requires storing all test results
// and that is expensive memory-wise. We therefore instead pick a set
// of points that we may be interested to use later and store
// only those points.
//
// The current design allows for common one and two-tailed intervals (e.g.
// 99.9%, 99%, 95%, 90%, etc.) to be calculated exactly and the remainder
// to be approximated.
//
// IMPORTANT: Do not change these points unless you handle data
// compatibility for old distributions that are already stored.
// MUST be in ascending order.
// MUST be symmetric around the middle, i.e.
// TailLikelihoods[i] = 1 - TailLikelihoods[(TailLikelihoodsLength-1) - i].
var TailLikelihoods = [TailLikelihoodsLength]float64{
	0.0005, // for 99.9% two-tailed Confidence Interval
	0.005,  // for 99% two-tailed Confidence Interval
	0.025,  // for 95% two-tailed Confidence Interval
	0.05,   // for 90% two-tailed Confidence Interval
	0.10,   // for 80% two-tailed Confidence Interval
	0.15,   // for 70% two-tailed Confidence Interval
	0.20,   // for 60% two-tailed Confidence Interval
	0.25,   // for 50% two-tailed Confidence Interval
	0.5,
	0.75,   // for 50% two-tailed Confidence Interval
	0.80,   // for 60% two-tailed Confidence Interval
	0.85,   // for 70% two-tailed Confidence Interval
	0.90,   // for 80% two-tailed Confidence Interval
	0.95,   // for 90% two-tailed Confidence Interval
	0.975,  // for 95% two-tailed Confidence Interval
	0.995,  // for 99% two-tailed Confidence Interval
	0.9995, // for 99.9% two-tailed Confidence Interval
}

// PositionDistribution represents the distribution of possible
// change point start (commit) positions. It is a quantization
// (i.e. sampling, compression of) the true distribution.
//
// To be precise, for a range of left-tail probilities x_0, x_1, ...
// (defined in TailLikelihoods above) it stores the upper bound
// source position k on the (unknown) true changepoint source position
// C such that P(C <= k) is approximately equal to x.
//
// X < 0.5 represents the left half of the distribution, and X > 0.5
// represents the right half. Note that the value of k that gives
// P(C <= k) closest to 0.5 is not necessarily the same as the nominal
// start position of the change point, in the same way that
// the median of a distribution is not the same as its mode (most
// likely value).
//
// k is always anchored to source position values with a test
// result and selected conservatively.
type PositionDistribution [TailLikelihoodsLength]int64

// ConfidenceInterval returns the (y*100)% two-tailed confidence interval
// for the change point start position.
//
// E.g. Y = 0.99 gives the 99% confidence interval (with left and right
// tails having probability ~ 0.005 each).
//
// The interpretation of (min, max) is as follows:
// There is at least a Y probability that the change which represents the start
// of the 'new' behaviour is between commit positions min exclusive and max inclusive.
//
// y shall be between 0.0 and 0.999.
func (d PositionDistribution) ConfidenceInterval(y float64) (min int64, max int64) {
	if len(d) != TailLikelihoodsLength {
		panic(fmt.Errorf("distribution has unexpected number of points; got %v, want %v", len(d), TailLikelihoodsLength))
	}
	maxY := 1.0 - (TailLikelihoods[0] * 2)
	if y < 0.0 || y > maxY {
		panic(fmt.Errorf("y must be between 0.0 and %v", maxY))
	}

	// E.g. 99% CI has tails of 0.005 on left and right.
	desiredTailProbability := (1.0 - y) / 2.0

	// Add 1e9 to avoid floating point imprecisions causing us to
	// fall into the next smaller entry. E.g. if we want the 99% CI,
	// we want to find the entry for 0.005 and not accidentally get the
	// one for 0.0025 because desiredTailProbability was 0.4999999999.
	leftTailIndex, found := slices.BinarySearch(TailLikelihoods[:], desiredTailProbability+1e-9)
	if leftTailIndex == 0 {
		// This indicates y > maxY, as then desiredTailProbability < TailLikelihoods[0].
		panic("should never be reached")
	}
	if !found {
		// The index returned above is the place the desired tail probability
		// should be inserted into the list to keep it in ascending order.
		// We want to find the next smaller entry (i.e. prefer expand the
		// confidence interval if there is no exact match).
		leftTailIndex -= 1
	}
	rightTailIndex := (TailLikelihoodsLength - 1) - leftTailIndex

	return d[leftTailIndex], d[rightTailIndex]
}

// PositionDistributionFromProto creates a PositionDistribution from its proto representation.
func PositionDistributionFromProto(v []int64) *PositionDistribution {
	if v == nil {
		// No distribution stored.
		return nil
	}
	if len(v) != TailLikelihoodsLength {
		panic(fmt.Errorf("distribution has unexpected number of points; got %v, want %v", len(v), TailLikelihoodsLength))
	}
	var previousValue int64
	var result PositionDistribution
	for i, delta := range v {
		previousValue += delta
		result[i] = previousValue
	}
	return &result
}

// Serialize serializes the position distribution to its proto representation.
func (d *PositionDistribution) Serialize() []int64 {
	if d == nil {
		// No distribution available.
		return nil
	}
	result := make([]int64, 0, len(d))
	var previousValue int64
	for _, v := range d {
		// Encode the delta from the previous position. This tends to compress
		// much better than recording absolute positions. For many changepoints
		// where the position is confident, many of these deltas will be zeroes.
		result = append(result, v-previousValue)
		previousValue = v
	}
	return result
}

// SimpleDistribution returns a distribution with the given center commit position and
// has 99% of data fall within the certain width from the center.
// Provided to simplify writing test cases that require a position distribution.
func SimpleDistribution(center int64, width int64) *PositionDistribution {
	var result PositionDistribution

	for i := range len(result) {
		if TailLikelihoods[i] < 0.005 {
			result[i] = center - width*2 - 1
		} else if TailLikelihoods[i] == 0.005 {
			result[i] = center - width - 1
		} else if TailLikelihoods[i] < 0.5 {
			result[i] = center - 1
		} else if TailLikelihoods[i] < 0.995 {
			result[i] = center
		} else if TailLikelihoods[i] == 0.995 {
			result[i] = center + width
		} else {
			result[i] = center + width*2
		}
	}
	return &result
}
