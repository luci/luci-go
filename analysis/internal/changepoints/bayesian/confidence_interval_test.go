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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/model"
)

func TestChangePointPositionConfidenceInterval(t *testing.T) {
	ftt.Run("ChangeChangepointPredictor", t, func(t *ftt.Test) {
		a := ChangepointPredictor{
			HasUnexpectedPrior: BetaDistribution{
				Alpha: 0.3,
				Beta:  0.5,
			},
			UnexpectedAfterRetryPrior: BetaDistribution{
				Alpha: 0.5,
				Beta:  0.5,
			},
		}
		t.Run("6 commit positions, each with 1 verdict", func(t *ftt.Test) {
			var (
				positions     = []int{1, 2, 3, 4, 5, 6}
				total         = []int{2, 2, 1, 1, 2, 2}
				hasUnexpected = []int{0, 0, 0, 1, 2, 2}
			)
			vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
			d := a.ChangePointPositionDistribution(vs)
			min, max := d.ConfidenceInterval(0.99)
			assert.Loosely(t, min, should.Equal(1)) // This should be treated as 'exclusive' value.
			assert.Loosely(t, max, should.Equal(5))
		})

		t.Run("4 commit positions, 2 verdict each", func(t *ftt.Test) {
			var (
				positions     = []int{1, 1, 2, 2, 3, 3, 4, 4}
				total         = []int{2, 2, 2, 2, 2, 2, 2, 2}
				hasUnexpected = []int{0, 0, 0, 0, 1, 1, 1, 1}
			)
			vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
			d := a.ChangePointPositionDistribution(vs)
			min, max := d.ConfidenceInterval(0.99)
			assert.Loosely(t, min, should.Equal(1))
			assert.Loosely(t, max, should.Equal(4))
		})

		t.Run("2 commit position with multiple verdicts each", func(t *ftt.Test) {
			var (
				positions     = []int{1, 1, 2, 2, 2, 2}
				total         = []int{3, 3, 1, 2, 3, 3}
				hasUnexpected = []int{0, 0, 0, 2, 3, 3}
			)
			vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
			d := a.ChangePointPositionDistribution(vs)
			min, max := d.ConfidenceInterval(0.99)
			// There is only one possible position for the changepoint.
			assert.Loosely(t, min, should.Equal(1)) // This should be treated as 'exclusive' value.
			assert.Loosely(t, max, should.Equal(2))
		})

		t.Run("Pass to flake transition", func(t *ftt.Test) {
			var (
				positions     = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
				total         = []int{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
				hasUnexpected = []int{0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
			)
			vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
			d := a.ChangePointPositionDistribution(vs)
			min, max := d.ConfidenceInterval(0.99)
			assert.Loosely(t, min, should.Equal(2))
			assert.Loosely(t, max, should.Equal(14))
		})

		t.Run("Flake to fail transition", func(t *ftt.Test) {
			var (
				positions     = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
				total         = []int{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
				hasUnexpected = []int{1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
			)
			vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
			d := a.ChangePointPositionDistribution(vs)
			min, max := d.ConfidenceInterval(0.99)
			assert.Loosely(t, min, should.Equal(2))
			assert.Loosely(t, max, should.Equal(14))
		})

		t.Run("(Fail, Pass after retry) to (Fail, Fail after retry)", func(t *ftt.Test) {
			var (
				positions            = []int{1, 2, 3, 4, 5, 6, 7, 8}
				total                = []int{2, 2, 2, 2, 2, 2, 2, 2}
				hasUnexpected        = []int{2, 2, 2, 2, 2, 2, 2, 2}
				retries              = []int{2, 2, 2, 2, 2, 2, 2, 2}
				unexpectedAfterRetry = []int{0, 0, 0, 0, 2, 2, 2, 2}
			)
			vs := inputbuffer.VerdictsWithRetriesRefs(positions, total, hasUnexpected, retries, unexpectedAfterRetry)
			d := a.ChangePointPositionDistribution(vs)
			min, max := d.ConfidenceInterval(0.99)
			assert.Loosely(t, min, should.Equal(3))
			assert.Loosely(t, max, should.Equal(6))
		})

		t.Run("(Fail, Fail after retry) to (Fail, Flaky on retry)", func(t *ftt.Test) {
			var (
				positions            = []int{1, 2, 3, 5, 5, 5, 7, 7}
				total                = []int{3, 3, 3, 1, 3, 3, 3, 3}
				hasUnexpected        = []int{3, 3, 3, 1, 3, 3, 3, 3}
				retries              = []int{3, 3, 3, 1, 3, 3, 3, 3}
				unexpectedAfterRetry = []int{3, 3, 3, 1, 0, 0, 1, 1}
			)
			vs := inputbuffer.VerdictsWithRetriesRefs(positions, total, hasUnexpected, retries, unexpectedAfterRetry)
			d := a.ChangePointPositionDistribution(vs)
			min, max := d.ConfidenceInterval(0.99)
			assert.Loosely(t, min, should.Equal(2))
			assert.Loosely(t, max, should.Equal(5))
		})

		t.Run("Pass to fail transition, sparse", func(t *ftt.Test) {
			var (
				positions     = []int{90, 100, 200, 210}
				total         = []int{5, 5, 5, 5}
				hasUnexpected = []int{0, 0, 5, 5}
			)
			vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
			t.Run("with consecutive positions", func(t *ftt.Test) {
				// With consecutive source positions.
				a.SourcePositionsConsecutive = true
				d := a.ChangePointPositionDistribution(vs)

				// The distribution reflects that we do not know where
				// in the range (100,200] the changepoint is.
				expectedDistribution := &model.PositionDistribution{100, 100, 100, 100, 100, 100, 100, 100, 200, 200, 200, 200, 200, 200, 200, 200, 200}
				assert.Loosely(t, d, should.Match(expectedDistribution))
			})
			t.Run("without consecutive positions", func(t *ftt.Test) {
				// Without consecutive source positions.
				a.SourcePositionsConsecutive = false
				d := a.ChangePointPositionDistribution(vs)

				// The distribution reflects that we do not know where
				// in the range (100,200] the changepoint is.
				expectedDistribution := &model.PositionDistribution{100, 100, 100, 100, 100, 100, 100, 100, 200, 200, 200, 200, 200, 200, 200, 200, 200}
				assert.Loosely(t, d, should.Match(expectedDistribution))
			})
		})
		t.Run("Pass to fail transition, sparse #2", func(t *ftt.Test) {
			var (
				positions     = []int{90, 100, 110, 120, 2000}
				total         = []int{5, 5, 2, 2, 2}
				hasUnexpected = []int{0, 0, 2, 2, 2}
			)
			vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
			t.Run("with consecutive positions", func(t *ftt.Test) {
				// Each source position corresponds to a new commit.
				a.SourcePositionsConsecutive = true
				d := a.ChangePointPositionDistribution(vs)

				// The distribution reflect the fact that there are
				// a lot of commits in the range (90,2000] which could
				// be the culprit. Although each individually has a low
				// probability, because there are so many it adds up and
				// skews the distribution.
				expectedDistribution := &model.PositionDistribution{90, 100, 100, 100, 100, 100, 100, 100, 110, 110, 110, 110, 110, 110, 2000, 2000, 2000}
				assert.Loosely(t, d, should.Match(expectedDistribution))
			})
			t.Run("without consecutive positions", func(t *ftt.Test) {
				// Source positions do not influence the distribution - they only define ordering.
				a.SourcePositionsConsecutive = false
				d := a.ChangePointPositionDistribution(vs)

				expectedDistribution := &model.PositionDistribution{90, 100, 100, 100, 100, 100, 100, 100, 110, 110, 110, 110, 110, 110, 110, 110, 120}
				assert.Loosely(t, d, should.Match(expectedDistribution))
			})
		})
		t.Run("Pass to fail transition, sparse #3", func(t *ftt.Test) {
			var (
				positions     = []int{90, 100, 1010, 1020, 1030}
				total         = []int{5, 5, 2, 2, 2}
				hasUnexpected = []int{0, 0, 2, 2, 2}
			)
			vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
			t.Run("with consecutive positions", func(t *ftt.Test) {
				// Each source position corresponds to a new commit.
				a.SourcePositionsConsecutive = true
				d := a.ChangePointPositionDistribution(vs)

				// The distribution reflect the fact that there are
				// a lot of commits in the range (100,1010] which could
				// be the culprit. This is also the most likely range for
				// culrpits, leading to 1010...1030 being in the far tail
				// of likely culprits.
				expectedDistribution := &model.PositionDistribution{100, 100, 100, 100, 100, 100, 100, 100, 1010, 1010, 1010, 1010, 1010, 1010, 1010, 1010, 1010}
				assert.Loosely(t, d, should.Match(expectedDistribution))
			})
			t.Run("without consecutive positions", func(t *ftt.Test) {
				// Source positions do not influence the distribution - they only define ordering.
				a.SourcePositionsConsecutive = false
				d := a.ChangePointPositionDistribution(vs)

				expectedDistribution := &model.PositionDistribution{90, 100, 100, 100, 100, 100, 100, 100, 1010, 1010, 1010, 1010, 1010, 1010, 1010, 1010, 1020}
				assert.Loosely(t, d, should.Match(expectedDistribution))
			})
		})

	})
}

func sum(totals []int) int {
	total := 0
	for _, v := range totals {
		total += v
	}
	return total
}

// Output as of Sept 2025 on AMD EPYC 7B13
// BenchmarkChangePointPositionConfidenceInterval-128     	    1905	    625073 ns/op	   49422 B/op	       5 allocs/op
func BenchmarkChangePointPositionConfidenceInterval(b *testing.B) {
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
		SourcePositionsConsecutive: true,
	}

	var vs []*inputbuffer.Run

	for i := 0; i <= 1000; i++ {
		vs = append(vs, &inputbuffer.Run{
			SourcePosition: int64(i),
			Expected:       inputbuffer.ResultCounts{PassCount: 1},
		})
	}
	for i := 1001; i < 2000; i++ {
		vs = append(vs, &inputbuffer.Run{
			SourcePosition: int64(i),
			Unexpected:     inputbuffer.ResultCounts{FailCount: 1},
		})
	}

	for i := 0; i < b.N; i++ {
		d := a.ChangePointPositionDistribution(vs)
		min, max := d.ConfidenceInterval(0.99)
		if min > 1001 || max < 1001 {
			panic("Invalid result")
		}
	}
}
