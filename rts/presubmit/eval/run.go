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
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Result is the result of evaluation.
type Result struct {
	Safety       Safety
	Efficiency   Efficiency
	TotalRecords int
}

type evalRun struct {
	Eval

	// internal mutable state
	res                      Result
	buf                      bytes.Buffer
	mostRecentProgressReport time.Time
	mu                       sync.Mutex
}

func (r *evalRun) run(ctx context.Context) error {
	// Init internal state.
	r.res = Result{}
	r.mostRecentProgressReport = time.Time{}

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	// Analyze safety.
	eg.Go(func() error {
		err := r.evaluateSafety(ctx)
		return errors.Annotate(err, "failed to evaluate safety").Err()
	})

	// Analyze efficiency.
	eg.Go(func() error {
		err := r.evaluateEfficiency(ctx)
		return errors.Annotate(err, "failed to evaluate efficiency").Err()
	})

	// Play back the history.
	eg.Go(func() error {
		err := r.History.Playback(ctx)
		r.res.TotalRecords = r.History.TotalRecords()
		return errors.Annotate(err, "failed to playback history").Err()
	})

	return eg.Wait()
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

func (r *evalRun) maybeReportProgress(ctx context.Context) {
	interval := r.ProgressReportInterval
	if interval <= 0 {
		interval = defaultProgressReportInterval
	}

	now := clock.Now(ctx)
	switch {
	case r.mostRecentProgressReport.IsZero():
		r.mostRecentProgressReport = now

	case now.After(r.mostRecentProgressReport.Add(interval)):
		logging.Infof(ctx, "%s", r.progressReportLine())
		r.mostRecentProgressReport = now
	}
}

func (r *evalRun) progressReportLine() string {
	r.buf.Reset()

	fmt.Fprintf(&r.buf, "change recall: %s", scoreString(r.res.Safety.ChangeRecall.Score()))
	fmt.Fprintf(&r.buf, " (%d data points)", r.res.Safety.ChangeRecall.TotalRejections)

	fmt.Fprintf(&r.buf, " | test recall: %s", scoreString(r.res.Safety.TestRecall.Score()))
	fmt.Fprintf(&r.buf, " (%d data points)", r.res.Safety.TestRecall.TotalFailures)

	fmt.Fprintf(&r.buf, " | efficiency: %s", scoreString(r.res.Efficiency.Score()))
	fmt.Fprintf(&r.buf, " (%d data points)", r.res.Efficiency.TestResults)

	return r.buf.String()
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
