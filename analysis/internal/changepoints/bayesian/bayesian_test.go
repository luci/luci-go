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
	"testing"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBayesianAnalysis(t *testing.T) {
	a := ChangepointPredictor{
		ChangepointLikelihood: 0.01,
		HasUnexpectedPrior: BetaDistribution{
			Alpha: 0.3,
			Beta:  0.5,
		},
		UnexpectedAfterRetryPrior: BetaDistribution{
			Alpha: 0.5,
			Beta:  0.5,
		},
	}
	Convey("Pass to fail transition 1", t, func() {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6}
			total         = []int{2, 2, 1, 1, 2, 2}
			hasUnexpected = []int{0, 0, 0, 1, 2, 2}
		)
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		changePoints := a.identifyChangePoints(vs)
		So(changePoints, ShouldResemble, []int{3})
	})

	Convey("Pass to fail transition 2", t, func() {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6}
			total         = []int{2, 2, 1, 1, 2, 2}
			hasUnexpected = []int{0, 0, 1, 1, 2, 2}
		)
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		changePoints := a.identifyChangePoints(vs)
		So(changePoints, ShouldResemble, []int{2})
	})

	Convey("Pass to flake transition", t, func() {
		var (
			positions     = []int{1, 1, 2, 2, 2, 2}
			total         = []int{3, 3, 1, 2, 3, 3}
			hasUnexpected = []int{0, 0, 0, 2, 3, 3}
		)
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		changePoints := a.identifyChangePoints(vs)
		So(changePoints, ShouldResemble, []int{2})
	})

	Convey("Pass to fail to pass transition", t, func() {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
			total         = []int{2, 2, 3, 2, 3, 1, 1, 2, 2, 3, 2, 3, 2, 2}
			hasUnexpected = []int{0, 0, 0, 0, 0, 1, 1, 2, 2, 3, 2, 3, 0, 0}
		)
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		changePoints := a.identifyChangePoints(vs)
		So(changePoints, ShouldResemble, []int{5, 12})
	})

	Convey("Pass to flake transition", t, func() {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
			total         = []int{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected = []int{0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
		)
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		changePoints := a.identifyChangePoints(vs)
		So(changePoints, ShouldResemble, []int{6})
	})

	Convey("Flake to fail transition", t, func() {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
			total         = []int{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected = []int{1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
		)
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		changePoints := a.identifyChangePoints(vs)
		So(changePoints, ShouldResemble, []int{6})
	})

	Convey("Pass consistently", t, func() {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6, 7, 8}
			total         = []int{2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected = []int{0, 0, 0, 0, 0, 0, 0, 0}
		)
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		changePoints := a.identifyChangePoints(vs)
		So(len(changePoints), ShouldEqual, 0)
	})

	Convey("Fail consistently", t, func() {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6, 7, 8}
			total         = []int{2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected = []int{2, 2, 2, 2, 2, 2, 2, 2}
		)
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		changePoints := a.identifyChangePoints(vs)
		So(len(changePoints), ShouldEqual, 0)
	})

	Convey("Flake", t, func() {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6, 7, 8}
			total         = []int{2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected = []int{1, 0, 1, 0, 0, 1, 0, 2}
		)
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		changePoints := a.identifyChangePoints(vs)
		So(len(changePoints), ShouldEqual, 0)
	})

	Convey("(Fail, Pass after retry) to (Fail, Fail after retry)", t, func() {
		var (
			positions            = []int{1, 2, 3, 4, 5, 6, 7, 8}
			total                = []int{2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected        = []int{2, 2, 2, 2, 2, 2, 2, 2}
			retries              = []int{2, 2, 2, 2, 2, 2, 2, 2}
			unexpectedAfterRetry = []int{0, 0, 0, 0, 2, 2, 2, 2}
		)
		vs := inputbuffer.VerdictsWithRetries(positions, total, hasUnexpected, retries, unexpectedAfterRetry)
		changePoints := a.identifyChangePoints(vs)
		So(changePoints, ShouldResemble, []int{4})
	})

	Convey("(Fail, Fail after retry) consistently", t, func() {
		var (
			positions            = []int{1, 2, 3, 4, 5, 6, 7, 8}
			total                = []int{2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected        = []int{2, 2, 2, 2, 2, 2, 2, 2}
			retries              = []int{2, 2, 2, 2, 2, 2, 2, 2}
			unexpectedAfterRetry = []int{2, 2, 2, 2, 2, 2, 2, 2}
		)
		vs := inputbuffer.VerdictsWithRetries(positions, total, hasUnexpected, retries, unexpectedAfterRetry)
		changePoints := a.identifyChangePoints(vs)
		So(len(changePoints), ShouldEqual, 0)
	})

	Convey("(Fail, Fail after retry) to (Fail, Flaky on retry)", t, func() {
		var (
			// The change point should be detected between commit positions 3 and 5.
			positions            = []int{1, 2, 3, 5, 5, 5, 7, 7}
			total                = []int{3, 3, 3, 1, 3, 3, 3, 3}
			hasUnexpected        = []int{3, 3, 3, 1, 3, 3, 3, 3}
			retries              = []int{3, 3, 3, 1, 3, 3, 3, 3}
			unexpectedAfterRetry = []int{3, 3, 3, 1, 0, 0, 1, 1}
		)
		vs := inputbuffer.VerdictsWithRetries(positions, total, hasUnexpected, retries, unexpectedAfterRetry)
		changePoints := a.identifyChangePoints(vs)
		So(changePoints, ShouldResemble, []int{3})
	})
}

// Output as of March 2023 on Intel Skylake CPU @ 2.00GHz:
// BenchmarkBayesianAnalysisConsistentPass-48    	   30054	     39879 ns/op	      18 B/op	       0 allocs/op
func BenchmarkBayesianAnalysisConsistentPass(b *testing.B) {
	a := ChangepointPredictor{
		ChangepointLikelihood: 0.01,
		HasUnexpectedPrior: BetaDistribution{
			Alpha: 0.3,
			Beta:  0.5,
		},
		UnexpectedAfterRetryPrior: BetaDistribution{
			Alpha: 0.5,
			Beta:  0.5,
		},
	}

	var vs []inputbuffer.PositionVerdict

	// Consistently passing test. This represents ~99% of tests.
	for i := 0; i < 2000; i++ {
		vs = append(vs, inputbuffer.PositionVerdict{
			CommitPosition:       i,
			IsSimpleExpectedPass: true,
		})
	}
	for i := 0; i < b.N; i++ {
		result := a.identifyChangePoints(vs)
		if len(result) != 0 {
			panic("unexpected result")
		}
	}
}

// Output as of March 2023 on Intel Skylake CPU @ 2.00GHz:
// BenchmarkBayesianAnalysisFlaky-48    	    1500	    796446 ns/op	     396 B/op	       0 allocs/op
func BenchmarkBayesianAnalysisFlaky(b *testing.B) {
	a := ChangepointPredictor{
		ChangepointLikelihood: 0.01,
		HasUnexpectedPrior: BetaDistribution{
			Alpha: 0.3,
			Beta:  0.5,
		},
		UnexpectedAfterRetryPrior: BetaDistribution{
			Alpha: 0.5,
			Beta:  0.5,
		},
	}
	// Flaky test.
	var vs []inputbuffer.PositionVerdict
	for i := 0; i < 2000; i++ {
		if i%2 == 0 {
			vs = append(vs, inputbuffer.PositionVerdict{
				CommitPosition:       i,
				IsSimpleExpectedPass: true,
			})
		} else {
			vs = append(vs, inputbuffer.PositionVerdict{
				CommitPosition: i,
				Details: inputbuffer.VerdictDetails{
					Runs: []inputbuffer.Run{
						{
							Expected: inputbuffer.ResultCounts{
								PassCount: 1,
							},
							Unexpected: inputbuffer.ResultCounts{
								FailCount: 1,
							},
						},
					},
				},
			})
		}
	}
	for i := 0; i < b.N; i++ {
		result := a.identifyChangePoints(vs)
		if len(result) != 0 {
			panic("unexpected result")
		}
	}
}
