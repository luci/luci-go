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
	ftt.Run("6 commit positions, each with 1 verdict", t, func(t *ftt.Test) {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6}
			total         = []int{2, 2, 1, 1, 2, 2}
			hasUnexpected = []int{0, 0, 0, 1, 2, 2}
		)
		vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
		d := a.ChangePointPositionDistribution(vs)
		min, max := d.ConfidenceInterval(0.99)
		assert.Loosely(t, min, should.Equal(2))
		assert.Loosely(t, max, should.Equal(5))
	})

	ftt.Run("4 commit positions, 2 verdict each", t, func(t *ftt.Test) {
		var (
			positions     = []int{1, 1, 2, 2, 3, 3, 4, 4}
			total         = []int{2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected = []int{0, 0, 0, 0, 1, 1, 1, 1}
		)
		vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
		d := a.ChangePointPositionDistribution(vs)
		min, max := d.ConfidenceInterval(0.99)
		assert.Loosely(t, min, should.Equal(2))
		assert.Loosely(t, max, should.Equal(4))
	})

	ftt.Run("2 commit position with multiple verdicts each", t, func(t *ftt.Test) {
		var (
			positions     = []int{1, 1, 2, 2, 2, 2}
			total         = []int{3, 3, 1, 2, 3, 3}
			hasUnexpected = []int{0, 0, 0, 2, 3, 3}
		)
		vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
		d := a.ChangePointPositionDistribution(vs)
		min, max := d.ConfidenceInterval(0.99)
		// There is only 1 possible position for change point
		assert.Loosely(t, min, should.Equal(2))
		assert.Loosely(t, max, should.Equal(2))
	})

	ftt.Run("Pass to flake transition", t, func(t *ftt.Test) {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
			total         = []int{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected = []int{0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
		)
		vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
		d := a.ChangePointPositionDistribution(vs)
		min, max := d.ConfidenceInterval(0.99)
		assert.Loosely(t, min, should.Equal(3))
		assert.Loosely(t, max, should.Equal(14))
	})

	ftt.Run("Flake to fail transition", t, func(t *ftt.Test) {
		var (
			positions     = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
			total         = []int{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
			hasUnexpected = []int{1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
		)
		vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
		d := a.ChangePointPositionDistribution(vs)
		min, max := d.ConfidenceInterval(0.99)
		assert.Loosely(t, min, should.Equal(3))
		assert.Loosely(t, max, should.Equal(14))
	})

	ftt.Run("(Fail, Pass after retry) to (Fail, Fail after retry)", t, func(t *ftt.Test) {
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
		assert.Loosely(t, min, should.Equal(4))
		assert.Loosely(t, max, should.Equal(6))
	})

	ftt.Run("(Fail, Fail after retry) to (Fail, Flaky on retry)", t, func(t *ftt.Test) {
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
		assert.Loosely(t, min, should.Equal(3))
		assert.Loosely(t, max, should.Equal(5))
	})

	ftt.Run("Pass to fail transition, sparse", t, func(t *ftt.Test) {
		var (
			positions     = []int{90, 100, 200, 210}
			total         = []int{5, 5, 5, 5}
			hasUnexpected = []int{0, 0, 5, 5}
		)
		vs := inputbuffer.VerdictRefs(positions, total, hasUnexpected)
		d := a.ChangePointPositionDistribution(vs)

		// The distribution reflects that we do not know where
		// in the range 101...200 the changepoint is.
		expectedDistribution := &model.PositionDistribution{101, 101, 101, 101, 101, 101, 101, 101, 150, 200, 200, 200, 200, 200, 200, 200, 200}
		assert.Loosely(t, d, should.Resemble(expectedDistribution))
	})
}

func sum(totals []int) int {
	total := 0
	for _, v := range totals {
		total += v
	}
	return total
}

// Output as of July 2024 on Intel Skylake CPU @ 2.00GHz:
// BenchmarkChangePointPositionConfidenceInterval-96    	    1270	    945067 ns/op	   66143 B/op	       6 allocs/op
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
	}

	var vs []*inputbuffer.Run

	for i := 0; i <= 1000; i++ {
		vs = append(vs, &inputbuffer.Run{
			CommitPosition: int64(i),
			Expected:       inputbuffer.ResultCounts{PassCount: 1},
		})
	}
	for i := 1001; i < 2000; i++ {
		vs = append(vs, &inputbuffer.Run{
			CommitPosition: int64(i),
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
