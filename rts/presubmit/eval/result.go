// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"fmt"
	"io"
	"math"
	"sort"
	"time"
)

// Result is the result of evaluation of a selection strategy.
type Result struct {
	// Thresholds are considered thresholds and their results.
	// Sorted by ascending ChangeScore.
	Thresholds []*Threshold

	// TotalRejections is the number of analyzed rejections.
	TotalRejections int
	// TotalTestFailures is the number of analyzed test failures.
	TotalTestFailures int
	// TotalDuration is the sum of analyzed test durations.
	TotalDuration time.Duration
}

// thresholdGrid is a 100x100 grid where each cell represents a distance/rank
// threshold. All cells in the same row have the same distance value, and
// all cells in the same column have the same rank value.
//
// The distance value of the row R is the minimal distance threshold required to
// achieve ChangeRecall score of (R+1)/100.0 on the training set, while ignoring
// rank threshold.
// For example, thresholdGrid[94][0].Value.Distance achieves 95% ChangeRecall
// on the training set.
//
// Similarly, rank value of the column C is the minimal rank threshold required
// to achieve ChangeRecall score of (C+1)/100.0 on to the training set,
// while ignoring distance threshold.
//
// The distances and ranks are computed independently of each other and then
// combined into this grid.
type thresholdGrid [100][100]Threshold

// init clears the grid and initializes the rows/columns with distance/rank
// percentiles in afs.
func (g *thresholdGrid) init(afs AffectednessSlice) (distancePercentiles []float64, rankPercentiles []int) {
	*g = thresholdGrid{}
	distancePercentiles, rankPercentiles = afs.quantiles(100)
	for row, distance := range distancePercentiles {
		for col, rank := range rankPercentiles {
			g[row][col].Value = Affectedness{Distance: distance, Rank: rank}
		}
	}
	return
}

// Threshold is distance and rank thresholds, as well as their results.
type Threshold struct {
	Value Affectedness

	// PreservedRejections is the number of rejections where at least one failed
	// test was selected.
	PreservedRejections int

	// PreservedTestFailures is the number of selected failed tests.
	PreservedTestFailures int

	// SavedDuration is the sum of test durations for skipped tests.
	SavedDuration time.Duration

	// ChangeRecall is the fraction of rejections that were preserved.
	// May be NaN.
	ChangeRecall float64

	// DistanceChangeRecall is the ChangeRecall achieved only by Value.Distance.
	DistanceChangeRecall float64

	// RankChangeRecall is the ChangeRecall achieved only by Value.Rank.
	RankChangeRecall float64

	// TestRecall is the fraction of test failures that were preserved.
	// May return NaN.
	TestRecall float64

	// Savings is the fraction of test duration that was cut.
	// May return NaN.
	Savings float64
}

// Slice returns a slice of thresholds, sorted by ChangeRecall and Savings.
// Returned elements are pointes to the grid.
func (g *thresholdGrid) Slice() []*Threshold {
	ret := make([]*Threshold, 0, 1e4)
	for row := 0; row < 100; row++ {
		for col := 0; col < 100; col++ {
			ret = append(ret, &g[row][col])
		}
	}

	sort.Slice(ret, func(i, j int) bool {
		if ret[i].ChangeRecall != ret[j].ChangeRecall {
			return ret[i].ChangeRecall < ret[i].ChangeRecall
		}
		return ret[i].Savings < ret[j].Savings
	})
	return ret
}

// Print prints the results to w.
func (r *Result) Print(w io.Writer, minChangeRecall float64) error {
	p := newPrinter(w)

	p.printf("ChangeRecall | Savings | TestRecall | Distance, ChangeRecall | Rank, ChangeRecall\n")
	p.printf("---------------------------------------------------------------------------------\n")
	for _, t := range r.Thresholds {
		if t.ChangeRecall < minChangeRecall {
			continue
		}
		p.printf(
			"%7s      | % 7s | %7s    | %6.3f            %3.0f%% | %-9d     %3.0f%%\n",
			scoreString(t.ChangeRecall),
			scoreString(t.Savings),
			scoreString(t.TestRecall),
			t.Value.Distance,
			t.DistanceChangeRecall*100,
			t.Value.Rank,
			t.RankChangeRecall*100,
		)
	}

	p.printf("\nbased on %d rejections, %d test failures, %s testing time\n", r.TotalRejections, r.TotalTestFailures, r.TotalDuration)

	return p.err
}

func scoreString(score float64) string {
	percentage := score * 100
	switch {
	case math.IsNaN(percentage):
		return "?"
	case percentage > 0 && percentage < 0.01:
		// Do not print it as 0.00%.
		return "<0.01%"
	case percentage > 99.99 && percentage < 100:
		// Do not print it as 100.00%.
		return ">99.99%"
	default:
		return fmt.Sprintf("%02.2f%%", percentage)
	}
}
