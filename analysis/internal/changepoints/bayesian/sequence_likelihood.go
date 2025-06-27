// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bayesian

import (
	"math"

	"gonum.org/v1/gonum/mathext"
)

// BetaDistribution specifies the parameters of a Beta distribution.
// See https://en.wikipedia.org/wiki/Beta_distribution.
//
// In this package, we use beta distribution as the prior for a
// test's propensity to produce unexpected results, because it
// simplifies the maths.
//
// Alpha = 1, Beta = 1 indicates an uninformative (uniform) prior.
type BetaDistribution struct {
	// Alpha parameter of Beta distribution.
	// Must be greater than 0.
	Alpha float64
	// Beta parameter of the Beta distribution.
	// Must be greater than 0.
	Beta float64
}

// SequenceLikelihood is used to compute the likelihood of observing
// a particular sequence of pass/fail results, assuming the failure rate
// p of the test remains constant over the sequence, and p is unknown but
// has the known prior Beta(alpha,beta) for some alpha and beta
// where alpha > 0 and beta > 0.
//
// See go/bayesian-changepoint-estimation for details.
type SequenceLikelihood struct {
	a float64
	b float64
	// Stores ln(Beta(a, b)), where Beta is Euler's beta function,
	// equal to (Gamma(a+b) / (Gamma(a) * Gamma(b)).
	logK float64
}

func NewSequenceLikelihood(prior BetaDistribution) SequenceLikelihood {
	if prior.Alpha <= 0 {
		panic("alpha must be greater than zero")
	}
	if prior.Beta <= 0 {
		panic("beta must be greater than zero")
	}
	return SequenceLikelihood{
		a:    prior.Alpha,
		b:    prior.Beta,
		logK: Lgamma(prior.Alpha+prior.Beta) - (Lgamma(prior.Alpha) + Lgamma(prior.Beta)),
	}
}

func Lgamma(x float64) float64 {
	value, _ := math.Lgamma(x)
	return value
}

// LogLikelihood returns the log-likelihood of observing a particular
// sequence of pass/fail results (Y_0, Y_1 ... Y_{n-1}) of a known length.
// i.e. returns log(P(Y)).
//
// See LogLikelihoodPartial for more details.
func (s SequenceLikelihood) LogLikelihood(x, n int) float64 {
	return s.LogLikelihoodPartial(x, n, 1.0)
}

// LogLikelihoodPartial returns the log-likelihood of:
//   - observing a particular binary sequence Y_0, Y_1 ... Y_{n-1},
//     knowing its length AND
//   - concurrently, the underlying proportion p (= P(Y_i = 1)) that led
//     to that sequence being less than or equal to maxProportion.
//
// i.e. the method returns log(P(Y AND p <= maxProportion)).
//
// The following assumptions are made:
//   - Y ~ BernoilliProcess(p, n), i.e. Y is n independent samples of a
//     bernoilli process with probability p of observing a 1.
//   - p ~ Beta(a, b), i.e. p is unknown but taken from a Beta distribution
//     with parameters a and b. If a = b = 1 is provided, the Beta distribution
//     collapses to the uniform (uninformative) prior.
//     In practice, where test failure rate is being modelled, a and b < 1
//     make more sense as tests tend towards being either consistently
//     failing or consistently passing.
//
// maxProportion must be between 0.0 and 1.0 (inclusive), and if it is 1.0,
// the method returns P(Y).
//
// While the method returns the probability of observing a precise sequence Y,
// the only statistics needed to evaluate this probability is x (the number
// of elements that are true) and n (the total), so these are the only values
// accepted as arguments. (As such, inputs are also constrained as 0 <= x <= n.)
//
// Log-likelihoods are returned, as for larger values of n (e.g. n > 100)
// the probabilities for any particular sequence can get very small.
// Working in log-probabilities avoids floating point precision issues.
//
// [1] https://en.wikipedia.org/wiki/Beta_distribution.
func (s SequenceLikelihood) LogLikelihoodPartial(x, n int, maxProportion float64) float64 {
	// The likelihood of observing a particular binary sequence Y
	// (consisting of elements Y_0, Y_1 ... Y_{n-1}), sampled from
	// a Bernoilli process with probability p is given by:
	//   P(Y | p) = p^x * (1-p)^(n-x)
	//
	// Where x is the total number of 'true' values in the sequence
	// and n is the total number of values in the sequence.
	//
	// However, we do not know p, only its prior, p ~ Beta(a, b).
	//
	// Thus the likelihood of observing a particular sequence
	// (not knowing p) is given by integrating over the possible
	// values of p as follows:
	//  P(Y) = {Integral over p, from 0 to 1} beta_pdf(p) * p^x * (1-p)^(n-x) dp
	//
	// Where beta_pdf(p) is the probability density function of the
	// Beta distribution, i.e.
	//   beta_pdf(p) = K * p^(a-1) * (1-p)^(b-1), where K = 1 / Beta(a, b).
	// And Beta is Euler's beta function.
	//
	// More generally, the joint probability of Y and some value of p
	// is as follows:
	// P(Y AND p <= maxProportion)
	//    = {Integral over p, from 0 to maxProportion} beta_pdf(p) * p^x * (1-p)^(n-x) dp
	//
	// Simplifying we have:
	//    = {Integral over p from 0 to maxProportion} (K * p^(a-1) * (1-p)^(b-1)) * p^x * (1-p)^(n-x) dp
	//          // Expanding beta_pdf()
	//    = K * {Integral over p from 0 to maxProportion} (p^(a-1) * (1-p)^(b-1)) * p^x * (1-p)^(n-x) dp
	//          // Moving the constant K out of the integral.
	//    = K * {Integral over p from 0 to maxProportion} p^(x+a-1) * (1-p)^(n-x+b-1) dp
	//          // using the rule that b^i * b^j = b^(i+j)
	//    = K * B(maxProportion; (x+a-1)+1, (n-x+b-1)+1)
	//          // Rewriting integral in terms of Euler's Incomplete Beta function, B.
	//    = K * B(maxProportion; x+a, n-x+b)
	//          // Simplifying arguments.
	// For a > 0, b > 0, x <= n, n >= 0, x >= 0.

	// Fortunately for us, Euler's Incomplete Beta function B is well-studied and
	// there are efficient algorithms for calculating it.
	//
	// The incomplete beta function B(k; c, d) can be rewritten in terms of the
	// Beta function, Beta, and the regularized incomplete beta function I_k:
	//   B(k; c, d) = Beta(c, d) * I_k(c, d).
	//
	// The function for I_k is available in gonum's mathext library as RegIncBeta.
	//
	// The Beta function itself can be rewritten in terms of the Gamma function:
	//   Beta(c, d) = Gamma(c) * Gamma(d) / Gamma(c + d)
	//
	// Where the Gamma function is built into go's standard math library.
	//
	// As we are going to worth with logarithms, we use the properties that
	// log(A*B) = log(A) + log(B), and that log(A/B) = log(A) - log(B).

	c := float64(x) + s.a
	d := float64(n-x) + s.b

	// Implement Euler's Beta using the Gamma function, using:
	// Beta(c, d) = Gamma(c) * Gamma(d) / Gamma(c + d).
	//
	// Note we are dealing with logarithms, so we use that:
	// log(A*B) = log(A) + log(B), and that log(A/B) = log(A) - log(B).
	logGammaC, _ := math.Lgamma(c)
	logGammaD, _ := math.Lgamma(d)
	logGammaCD, _ := math.Lgamma(c + d)
	logBeta := logGammaC + logGammaD - logGammaCD

	logRegIncBeta := 0.0
	if maxProportion < 1.0 {
		logRegIncBeta = math.Log(mathext.RegIncBeta(c, d, maxProportion))
	}

	// K * Beta(c, d) * I_{maxProportion}(c, d)
	return s.logK + logBeta + logRegIncBeta
}

func addLogLikelihoods(x []float64) float64 {
	if len(x) == 1 {
		return x[0]
	}
	maxValue := -math.MaxFloat64
	for i := range x {
		if x[i] > maxValue {
			maxValue = x[i]
		}
	}

	// Rearrange:
	//
	//   log(e^x + e^y + e^z + ...)
	// = log(e^a * (e^(x-a) + e^(y-a) + e^(z-a))),
	//   letting a = max(x, y, z, ...)
	// = log(e^a) + log(e^(x-a) + e^(y-a) + e^(z-a))
	// = a + log(e^(x-a) + e^(y-a) + e^(z-a))
	//
	// This keeps us running into limits of float64s
	// (e.g. max exponent of 10^(+/-308)) when
	// adding very small (or very large) values.
	sum := 0.0
	for i := range x {
		sum += math.Exp(x[i] - maxValue)
	}
	return maxValue + math.Log(sum)
}
