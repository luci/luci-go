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

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Result is the result of evaluation of a selection strategy.
type Result struct {
	Thresholds ThresholdGrid

	TrainingRecords int
	EvalRecords     int

	// TotalRejections is the total number of analyzed rejections.
	TotalRejections int
	// TotalTestFailures is the total number of analyzed test failures.
	TotalTestFailures int
	// TotalDuration is the sum of test durations in the analyzed data.
	TotalDuration time.Duration

	// LostRejections are the rejections that would not be preserved
	// by the candidate strategy, i.e. the bad patchsets would land.
	// The strategy did not select any of the failed tests in these rejections.
	//
	// Ideally this slice is empty.
	WorstRejections []*evalpb.Rejection
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

// Threshold is distance and rank thesholds, as well as their results on the
// eval set.
type Threshold struct {
	Value Affectedness

	PreservedRejections   int
	PreservedTestFailures int
	SavedDuration         time.Duration
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
		p.printf("Evaluation failed: no rejections in the eval set")
		return nil
	case r.TotalDuration == 0:
		p.printf("Evaluation failed: the total test duration is 0 in the eval set")
		return nil
	}

	// TODO(nodir): compute overfitting.

	type entry struct {
		Threshold
		Scores
		ExpectedDistanceChangeRecall float64
		ExpectedRankChangeRecall     float64
	}
	entries := map[float64]entry{}
	for row := 0; row < 100; row++ {
		for col := 0; col < 100; col++ {
			t := r.Thresholds[row][col]
			s := r.TresholdScores(t)
			if e, ok := entries[s.ChangeRecall]; !ok || s.Savings > e.Savings {
				entries[s.ChangeRecall] = entry{
					Threshold:                    t,
					Scores:                       s,
					ExpectedDistanceChangeRecall: float64(row+1) / 100.0,
					ExpectedRankChangeRecall:     float64(col+1) / 100.0,
				}
			}
		}
	}

	keys := make([]float64, 0, len(entries))
	for score := range entries {
		keys = append(keys, score)
	}
	sort.Float64s(keys)

	p.printf("ChangeRecall | Savings | TestRecall | Distance (expected ChangeRecall) | Rank (expected ChangeRecall)\n")
	p.printf("-----------------------------------------------------\n")
	for _, key := range keys {
		e := entries[key]
		// TODO(nodir): change padding. it must be on the right
		p.printf(
			"%7s      | %7s | %10s | %2.3f (%s)       | %d (%s)\n",
			scoreString(e.ChangeRecall),
			scoreString(e.Savings),
			scoreString(e.TestRecall),
			e.Value.Distance,
			scoreString(e.ExpectedDistanceChangeRecall),
			e.Value.Rank,
			scoreString(e.ExpectedRankChangeRecall),
		)
	}
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
