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
	"time"

	"go.chromium.org/luci/rts"
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

// Threshold is distance and rank thresholds, as well as their results.
type Threshold struct {
	Value rts.Affectedness

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
