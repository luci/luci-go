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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSequenceLikelihood(t *testing.T) {
	ftt.Run("Uniform prior", t, func(t *ftt.Test) {
		prior := BetaDistribution{
			Alpha: 1.0,
			Beta:  1.0,
		}
		sl := NewSequenceLikelihood(prior)

		t.Run("Empty sequence", func(t *ftt.Test) {
			// The probability of observing the empty sequence
			// knowing the sequence length is 0 is 1.0.
			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 0)), should.AlmostEqual(1.0))
		})
		t.Run("Sequence of length one", func(t *ftt.Test) {
			// If the sequence is of length one, and the prior
			// is not biased one way or the other, the probability
			// of observing a pass or a fail should be the same
			// and add up to 1.0.
			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 1)), should.AlmostEqual(0.5))
			assert.Loosely(t, math.Exp(sl.LogLikelihood(1, 1)), should.AlmostEqual(0.5))
		})
		t.Run("Sequence of length two", func(t *ftt.Test) {
			// The following identities are harder to explain as
			// the sequence likelihoods are obtained by integrating
			// over all possible test failure rates.
			// However, we know the probability of observing all
			// sequences must add up to one.

			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 2)), should.AlmostEqual(0.3333333333333333))
			// There are two sequences that have one pass and one failure.
			assert.Loosely(t, 2*math.Exp(sl.LogLikelihood(1, 2)), should.AlmostEqual(0.3333333333333333))
			assert.Loosely(t, math.Exp(sl.LogLikelihood(2, 2)), should.AlmostEqual(0.3333333333333333))
		})
		t.Run("Sequence of length three", func(t *ftt.Test) {
			// Coefficients (1, 3, 3, 1) from Pascal's triangle.
			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 3)), should.AlmostEqual(0.25))
			assert.Loosely(t, 3*math.Exp(sl.LogLikelihood(1, 3)), should.AlmostEqual(0.25))
			assert.Loosely(t, 3*math.Exp(sl.LogLikelihood(2, 3)), should.AlmostEqual(0.25))
			assert.Loosely(t, math.Exp(sl.LogLikelihood(3, 3)), should.AlmostEqual(0.25))
		})
		t.Run("Sequence of length four", func(t *ftt.Test) {
			// Coefficients (1, 4, 6, 4, 1) from Pascal's triangle.
			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 4)), should.AlmostEqual(0.20))
			assert.Loosely(t, 4*math.Exp(sl.LogLikelihood(1, 4)), should.AlmostEqual(0.20))
			assert.Loosely(t, 6*math.Exp(sl.LogLikelihood(2, 4)), should.AlmostEqual(0.20))
			assert.Loosely(t, 4*math.Exp(sl.LogLikelihood(3, 4)), should.AlmostEqual(0.20))
			assert.Loosely(t, math.Exp(sl.LogLikelihood(4, 4)), should.AlmostEqual(0.20))
		})
	})
	ftt.Run("Non-uniform prior", t, func(t *ftt.Test) {
		// This prior is biased towards the test either
		// passing or failing consistently, with the
		// passing consistently case more likely.
		prior := BetaDistribution{
			Alpha: 0.3,
			Beta:  0.5,
		}
		sl := NewSequenceLikelihood(prior)

		t.Run("Empty sequence", func(t *ftt.Test) {
			// The probability of observing the empty sequence
			// knowing the sequence length is 0 is 1.0.
			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 0)), should.AlmostEqual(1.0))
		})
		t.Run("Sequence of length one", func(t *ftt.Test) {
			// Verify sequences with fewer failures are more likely
			// and the probabilities add up to 1.0.
			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 1)), should.AlmostEqual(0.625))
			assert.Loosely(t, math.Exp(sl.LogLikelihood(1, 1)), should.AlmostEqual(0.375))
		})
		t.Run("Sequence of length two", func(t *ftt.Test) {
			// The following results were not verified with respect to ground
			// truth, but we did verify the probabilities added up to 1.0
			// and shape is expected.

			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 2)), should.AlmostEqual(0.5208333333333335))
			// There are two sequences that have one pass
			// and one failure.
			assert.Loosely(t, 2*math.Exp(sl.LogLikelihood(1, 2)), should.AlmostEqual(0.2083333333333334))
			assert.Loosely(t, math.Exp(sl.LogLikelihood(2, 2)), should.AlmostEqual(0.2708333333333333))
		})
		t.Run("Sequence of length three", func(t *ftt.Test) {
			// The following results were not verified with respect to ground
			// truth, but we did verify the probabilities added up to 1.0
			// and shape is expected.

			// Coefficients (1, 3, 3, 1) from Pascal's triangle.
			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 3)), should.AlmostEqual(0.465029761904762))
			assert.Loosely(t, 3*math.Exp(sl.LogLikelihood(1, 3)), should.AlmostEqual(0.16741071428571436))
			assert.Loosely(t, 3*math.Exp(sl.LogLikelihood(2, 3)), should.AlmostEqual(0.14508928571428578))
			assert.Loosely(t, math.Exp(sl.LogLikelihood(3, 3)), should.AlmostEqual(0.2224702380952381))
		})
		t.Run("Sequence of length four", func(t *ftt.Test) {
			// The following results were not verified with respect to ground
			// truth, but we did verify the probabilities added up to 1.0
			// and shape is expected.

			// Coefficients (1, 4, 6, 4, 1) from Pascal's triangle.
			assert.Loosely(t, math.Exp(sl.LogLikelihood(0, 4)), should.AlmostEqual(0.4283168859649124))
			assert.Loosely(t, 4*math.Exp(sl.LogLikelihood(1, 4)), should.AlmostEqual(0.14685150375939848))
			assert.Loosely(t, 6*math.Exp(sl.LogLikelihood(2, 4)), should.AlmostEqual(0.11454417293233085))
			assert.Loosely(t, 4*math.Exp(sl.LogLikelihood(3, 4)), should.AlmostEqual(0.11708959899749374))
			assert.Loosely(t, math.Exp(sl.LogLikelihood(4, 4)), should.AlmostEqual(0.19319783834586465))
		})
	})
}

func TestAddLogLikelihood(t *testing.T) {
	ftt.Run("AddLogLikelihood", t, func(t *ftt.Test) {
		t.Run("One element", func(t *ftt.Test) {
			assert.Loosely(t, addLogLikelihoods([]float64{math.Log(0.1)}), should.Equal(math.Log(0.1)))
		})

		t.Run("Many elements", func(t *ftt.Test) {
			assert.Loosely(t, addLogLikelihoods([]float64{math.Log(0.1), math.Log(0.2), math.Log(0.3)}), should.AlmostEqual(math.Log(0.6)))
		})

		t.Run("Many small elements", func(t *ftt.Test) {
			var eles = make([]float64, 10)
			for i := range 10 {
				eles[i] = math.Log(0.0000001)
			}
			assert.Loosely(t, addLogLikelihoods(eles), should.AlmostEqual(math.Log(0.000001)))
		})
	})
}
