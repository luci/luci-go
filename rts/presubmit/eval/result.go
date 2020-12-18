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
	Thresholds ThresholdGrid

	// TotalRejections is the number of analyzed rejections.
	TotalRejections int
	// TotalTestFailures is the number of analyzed test failures.
	TotalTestFailures int
	// TotalDuration is the sum of analyzed test durations.
	TotalDuration time.Duration
}

// ThresholdGrid is a 100x100 grid where each cell represents a distance/rank
// threshold. All cells in the same row have the same distance value, and
// all cells in the same column have the same rank value.
//
// The distance value of the row R is the minimal distance threshold required to
// achieve ChangeRecall score of (R+1)/100.0 on the training set, while ignoring
// rank threshold.
// For example, ThresholdGrid[94][0].Value.Distance achieves 95% ChangeRecall
// on the training set.
//
// Similarly, rank value of the column C is the minimal rank threshold required
// to achieve ChangeRecall score of (C+1)/100.0 on to the training set,
// while ignoring distance threshold.
//
// The distances and ranks are computed independently of each other and then
// combined into this grid.
type ThresholdGrid [100][100]Threshold

// init clears the grid and initializes the rows/columns with distance/rank
// percentiles in afs.
func (g *ThresholdGrid) init(afs AffectednessSlice) (distancePercentiles []float64, rankPercentiles []int) {
	*g = ThresholdGrid{}
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
}

// Scores are safety/efficiency scores computed for a particular threshold.
type Scores struct {
	// ChangeRecall is the fraction of rejections that were preserved.
	// May be NaN.
	ChangeRecall float64
	// TestRecall is the fraction of test failures that were preserved.
	// May return NaN.
	TestRecall float64
	// Savings is the fraction of test duration that was cut.
	// May return NaN.
	Savings float64
}

// TresholdScores returns safety/efficiency scores for a particular threshold.
func (r *Result) TresholdScores(t Threshold) Scores {
	s := Scores{
		ChangeRecall: math.NaN(),
		TestRecall:   math.NaN(),
		Savings:      math.NaN(),
	}
	if r.TotalRejections > 0 {
		s.ChangeRecall = float64(t.PreservedRejections) / float64(r.TotalRejections)
	}
	if r.TotalTestFailures > 0 {
		s.TestRecall = float64(t.PreservedTestFailures) / float64(r.TotalTestFailures)
	}
	if r.TotalDuration > 0 {
		s.Savings = float64(t.SavedDuration) / float64(r.TotalDuration)
	}
	return s
}

// Print prints the results to w.
func (r *Result) Print(w io.Writer, minChangeRecall float64) error {
	p := newPrinter(w)

	switch {
	case r.TotalRejections == 0:
		p.printf("Evaluation failed: no rejections data")
		return nil
	case r.TotalDuration == 0:
		p.printf("Evaluation failed: the total test duration is 0")
		return nil
	}

	type changeRecall struct {
		Threshold
		Scores
		DistanceChangeRecall float64
		RankChangeRecall     float64
	}
	changeRecalls := map[float64]changeRecall{}
	for row := 0; row < 100; row++ {
		for col := 0; col < 100; col++ {
			t := r.Thresholds[row][col]
			s := r.TresholdScores(t)
			if s.ChangeRecall < minChangeRecall {
				continue
			}
			// Ignore the threshold if it is strictly worse than what we already have.
			if cr, ok := changeRecalls[s.ChangeRecall]; !ok || s.Savings > cr.Savings {
				changeRecalls[s.ChangeRecall] = changeRecall{
					Threshold:            t,
					Scores:               s,
					DistanceChangeRecall: float64(row+1) / 100.0,
					RankChangeRecall:     float64(col+1) / 100.0,
				}
			}
		}
	}

	keys := make([]float64, 0, len(changeRecalls))
	for score := range changeRecalls {
		keys = append(keys, score)
	}
	sort.Float64s(keys)

	p.printf("ChangeRecall | Savings | TestRecall | Distance, ChangeRecall | Rank,     ChangeRecall\n")
	p.printf("-------------------------------------------------------------------------------------\n")
	for _, key := range keys {
		e := changeRecalls[key]
		p.printf(
			"%7s      | % 7s | %7s    | %6.3f    %7s      | %-9d %7s\n",
			scoreString(e.ChangeRecall),
			scoreString(e.Savings),
			scoreString(e.TestRecall),
			e.Value.Distance,
			scoreString(e.DistanceChangeRecall),
			e.Value.Rank,
			scoreString(e.RankChangeRecall),
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
