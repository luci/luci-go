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

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Result is the result of evaluation.
type Result struct {
	Safety       Safety
	Efficiency   Efficiency
	TotalRecords int
}

// Safety is how safe the candidate strategy is.
// A safe strategy does not let bad CLs pass CQ.
type Safety struct {
	ChangeRecall ChangeRecall
	TestRecall   TestRecall
}

// ChangeRecall represents the fraction of code change rejections
// that were preserved by the candidate strategy.
type ChangeRecall struct {
	// TotalRejections is the total number of analyzed rejections.
	TotalRejections int

	// LostRejections are the rejections that would not be preserved
	// by the candidate strategy, i.e. the bad patchsets would land.
	// The strategy did not select any of the failed tests in these rejections.
	//
	// Ideally this slice is empty.
	LostRejections []*evalpb.Rejection
}

// TestRecall represents the fraction of test failures
// that were preserved by the candidate strategy.
// If a test is not selected by the strategy, then its failure is not
// preserved.
type TestRecall struct {
	// TotalFailures is the total number of analyzed test failures.
	TotalFailures int

	// LostFailures are the failures which the candidate strategy did not preserve.
	LostFailures []LostTestFailure
}

// LostTestFailure is a failure of a test that the candidate strategy did not select.
type LostTestFailure struct {
	Rejection   *evalpb.Rejection
	TestVariant *evalpb.TestVariant
}

// Efficiency is result of evaluation how much compute time the candidate
// strategy could save.
type Efficiency struct {
	// TestResults is the number of analyzed test results.
	TestResults int

	// SampleDuration is the sum of test durations in the analyzed data sample.
	SampleDuration time.Duration

	// ForecastDuration is the sum of test durations for tests selected by the
	// strategy. It is a value between 0 and SampleDuration.
	// The lower the number the better.
	ForecastDuration time.Duration
}

// Print prints the results to w.
func (r *Result) Print(w io.Writer) error {
	p := newPrinter(w)
	pf := p.printf

	pf("Change recall:\n")
	cr := r.Safety.ChangeRecall
	p.Level++
	switch {
	case cr.TotalRejections == 0:
		pf("Evaluation failed: rejections not found\n")

	default:
		pf("Score: %s\n", scoreString(cr.Score()))
		pf("Rejections:           %d\n", cr.TotalRejections)
		pf("Rejections preserved: %d\n", cr.preserved())
	}
	p.Level--

	pf("Test recall:\n")
	tr := r.Safety.TestRecall
	p.Level++
	switch {
	case tr.TotalFailures == 0:
		pf("Evaluation failed: rejections not found\n")

	default:
		pf("Score: %s\n", scoreString(tr.Score()))
		pf("Test failures:           %d\n", tr.TotalFailures)
		pf("Test failures preserved: %d\n", tr.preserved())
	}
	p.Level--

	pf("Efficiency:\n")
	p.Level++
	if r.Efficiency.SampleDuration == 0 {
		pf("Evaluation failed: no test results with duration\n")
	} else {
		pf("Saved: %s\n", scoreString(r.Efficiency.Score()))
		pf("Test results analyzed: %d\n", r.Efficiency.TestResults)
		pf("Compute time in the sample: %s\n", r.Efficiency.SampleDuration)
		pf("Forecasted compute time:    %s\n", r.Efficiency.ForecastDuration)
	}
	p.Level--

	pf("Total records: %d\n", r.TotalRecords)
	return p.err
}

func (r *Result) oneLine(w io.Writer) error {
	p := newPrinter(w)
	p.printf("change recall: %s", scoreString(r.Safety.ChangeRecall.Score()))
	p.printf(" (%d data points)", r.Safety.ChangeRecall.TotalRejections)

	p.printf(" | test recall: %s", scoreString(r.Safety.TestRecall.Score()))
	p.printf(" (%d data points)", r.Safety.TestRecall.TotalFailures)

	p.printf(" | efficiency: %s", scoreString(r.Efficiency.Score()))
	p.printf(" (%d data points)", r.Efficiency.TestResults)
	return p.err
}

func (r *ChangeRecall) preserved() int {
	return r.TotalRejections - len(r.LostRejections)
}

// Score returns the fraction of rejections that were preserved.
// May return NaN.
func (r *ChangeRecall) Score() float64 {
	if r.TotalRejections == 0 {
		return math.NaN()
	}
	return float64(r.preserved()) / float64(r.TotalRejections)
}

func (r *TestRecall) preserved() int {
	return r.TotalFailures - len(r.LostFailures)
}

// Score returns the fraction of test failures that were preserved.
// May return NaN.
func (r *TestRecall) Score() float64 {
	if r.TotalFailures == 0 {
		return math.NaN()
	}
	return float64(r.preserved()) / float64(r.TotalFailures)
}

// Score returns the efficiency score.
// May return NaN.
func (e *Efficiency) Score() float64 {
	if e.SampleDuration == 0 {
		return math.NaN()
	}
	saved := e.SampleDuration - e.ForecastDuration
	return float64(saved) / float64(e.SampleDuration)
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
