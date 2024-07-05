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

	. "github.com/smartystreets/goconvey/convey"
)

func TestSequenceLikelihood(t *testing.T) {
	Convey("Uniform prior", t, func() {
		prior := BetaDistribution{
			Alpha: 1.0,
			Beta:  1.0,
		}
		sl := NewSequenceLikelihood(prior)

		Convey("Empty sequence", func() {
			// The probability of observing the empty sequence
			// knowing the sequence length is 0 is 1.0.
			So(math.Exp(sl.LogLikelihood(0, 0)), ShouldAlmostEqual, 1.0)
		})
		Convey("Sequence of length one", func() {
			// If the sequence is of length one, and the prior
			// is not biased one way or the other, the probability
			// of observing a pass or a fail should be the same
			// and add up to 1.0.
			So(math.Exp(sl.LogLikelihood(0, 1)), ShouldAlmostEqual, 0.5)
			So(math.Exp(sl.LogLikelihood(1, 1)), ShouldAlmostEqual, 0.5)
		})
		Convey("Sequence of length two", func() {
			// The following identities are harder to explain as
			// the sequence likelihoods are obtained by integrating
			// over all possible test failure rates.
			// However, we know the probability of observing all
			// sequences must add up to one.

			So(math.Exp(sl.LogLikelihood(0, 2)), ShouldAlmostEqual, 0.3333333333333333)
			// There are two sequences that have one pass and one failure.
			So(2*math.Exp(sl.LogLikelihood(1, 2)), ShouldAlmostEqual, 0.3333333333333333)
			So(math.Exp(sl.LogLikelihood(2, 2)), ShouldAlmostEqual, 0.3333333333333333)
		})
		Convey("Sequence of length three", func() {
			// Coefficients (1, 3, 3, 1) from Pascal's triangle.
			So(math.Exp(sl.LogLikelihood(0, 3)), ShouldAlmostEqual, 0.25)
			So(3*math.Exp(sl.LogLikelihood(1, 3)), ShouldAlmostEqual, 0.25)
			So(3*math.Exp(sl.LogLikelihood(2, 3)), ShouldAlmostEqual, 0.25)
			So(math.Exp(sl.LogLikelihood(3, 3)), ShouldAlmostEqual, 0.25)
		})
		Convey("Sequence of length four", func() {
			// Coefficients (1, 4, 6, 4, 1) from Pascal's triangle.
			So(math.Exp(sl.LogLikelihood(0, 4)), ShouldAlmostEqual, 0.20)
			So(4*math.Exp(sl.LogLikelihood(1, 4)), ShouldAlmostEqual, 0.20)
			So(6*math.Exp(sl.LogLikelihood(2, 4)), ShouldAlmostEqual, 0.20)
			So(4*math.Exp(sl.LogLikelihood(3, 4)), ShouldAlmostEqual, 0.20)
			So(math.Exp(sl.LogLikelihood(4, 4)), ShouldAlmostEqual, 0.20)
		})
	})
	Convey("Non-uniform prior", t, func() {
		// This prior is biased towards the test either
		// passing or failing consistently, with the
		// passing consistently case more likely.
		prior := BetaDistribution{
			Alpha: 0.3,
			Beta:  0.5,
		}
		sl := NewSequenceLikelihood(prior)

		Convey("Empty sequence", func() {
			// The probability of observing the empty sequence
			// knowing the sequence length is 0 is 1.0.
			So(math.Exp(sl.LogLikelihood(0, 0)), ShouldAlmostEqual, 1.0)
		})
		Convey("Sequence of length one", func() {
			// Verify sequences with fewer failures are more likely
			// and the probabilities add up to 1.0.
			So(math.Exp(sl.LogLikelihood(0, 1)), ShouldAlmostEqual, 0.625)
			So(math.Exp(sl.LogLikelihood(1, 1)), ShouldAlmostEqual, 0.375)
		})
		Convey("Sequence of length two", func() {
			// The following results were not verified with respect to ground
			// truth, but we did verify the probabilities added up to 1.0
			// and shape is expected.

			So(math.Exp(sl.LogLikelihood(0, 2)), ShouldAlmostEqual, 0.520833333333333)
			// There are two sequences that have one pass
			// and one failure.
			So(2*math.Exp(sl.LogLikelihood(1, 2)), ShouldAlmostEqual, 0.208333333333333)
			So(math.Exp(sl.LogLikelihood(2, 2)), ShouldAlmostEqual, 0.2708333333333333)
		})
		Convey("Sequence of length three", func() {
			// The following results were not verified with respect to ground
			// truth, but we did verify the probabilities added up to 1.0
			// and shape is expected.

			// Coefficients (1, 3, 3, 1) from Pascal's triangle.
			So(math.Exp(sl.LogLikelihood(0, 3)), ShouldAlmostEqual, 0.465029761904762)
			So(3*math.Exp(sl.LogLikelihood(1, 3)), ShouldAlmostEqual, 0.16741071428571436)
			So(3*math.Exp(sl.LogLikelihood(2, 3)), ShouldAlmostEqual, 0.14508928571428578)
			So(math.Exp(sl.LogLikelihood(3, 3)), ShouldAlmostEqual, 0.2224702380952381)
		})
		Convey("Sequence of length four", func() {
			// The following results were not verified with respect to ground
			// truth, but we did verify the probabilities added up to 1.0
			// and shape is expected.

			// Coefficients (1, 4, 6, 4, 1) from Pascal's triangle.
			So(math.Exp(sl.LogLikelihood(0, 4)), ShouldAlmostEqual, 0.4283168859649124)
			So(4*math.Exp(sl.LogLikelihood(1, 4)), ShouldAlmostEqual, 0.14685150375939848)
			So(6*math.Exp(sl.LogLikelihood(2, 4)), ShouldAlmostEqual, 0.11454417293233085)
			So(4*math.Exp(sl.LogLikelihood(3, 4)), ShouldAlmostEqual, 0.11708959899749374)
			So(math.Exp(sl.LogLikelihood(4, 4)), ShouldAlmostEqual, 0.19319783834586465)
		})
	})
}

func TestAddLogLikelihood(t *testing.T) {
	Convey("AddLogLikelihood", t, func() {
		Convey("One element", func() {
			So(addLogLikelihoods([]float64{math.Log(0.1)}), ShouldEqual, math.Log(0.1))
		})

		Convey("Many elements", func() {
			So(addLogLikelihoods([]float64{math.Log(0.1), math.Log(0.2), math.Log(0.3)}), ShouldAlmostEqual, math.Log(0.6))
		})

		Convey("Many small elements", func() {
			var eles = make([]float64, 10)
			for i := 0; i < 10; i++ {
				eles[i] = math.Log(0.0000001)
			}
			So(addLogLikelihoods(eles), ShouldAlmostEqual, math.Log(0.000001))
		})

	})
}
