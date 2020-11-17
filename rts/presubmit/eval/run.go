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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Result is the result of evaluation.
type Result struct {
	Safety       Safety
	Efficiency   Efficiency
	TotalRecords int
}

type evalRun struct {
	Eval

	auth   *auth.Authenticator
	gerrit *gerritClient

	// internal mutable state
	res                      Result
	buf                      bytes.Buffer
	mostRecentProgressReport time.Time
	mu                       sync.Mutex

	rejectionC chan *evalpb.Rejection
	durationC  chan *evalpb.TestDuration
}

func (r *evalRun) run(ctx context.Context) error {
	if err := r.Init(ctx); err != nil {
		return err
	}

	// Init internal state.
	r.res = Result{}
	r.mostRecentProgressReport = time.Time{}
	r.rejectionC = make(chan *evalpb.Rejection)
	r.durationC = make(chan *evalpb.TestDuration)

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
		defer func() {
			close(r.rejectionC)
			close(r.durationC)
			r.History.Close()
		}()
		err := r.playbackHistory(ctx)
		return errors.Annotate(err, "failed to playback history").Err()
	})

	return eg.Wait()
}

// playbackHistory reads records from r.Eval.History and dispatches them
// to r.rejectionC and r.durationC.
func (r *evalRun) playbackHistory(ctx context.Context) error {
	curRej := &evalpb.Rejection{}
	for {
		rec, err := r.History.Read()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return errors.Annotate(err, "failed to read history").Err()
		}

		r.mu.Lock()
		r.res.TotalRecords++
		r.mu.Unlock()

		// Send the record to the appropriate channel.
		switch data := rec.Data.(type) {

		case *evalpb.Record_RejectionFragment:
			proto.Merge(curRej, data.RejectionFragment.Rejection)
			if data.RejectionFragment.Terminal {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case r.rejectionC <- curRej:
					// Start a new rejection.
					curRej = &evalpb.Rejection{}
				}
			}

		case *evalpb.Record_TestDuration:
			select {
			case <-ctx.Done():
				return ctx.Err()
			case r.durationC <- data.TestDuration:
			}

		default:
			panic(fmt.Sprintf("unexpected record %s", rec))
		}
	}
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

	case cr.EligibleRejections == 0:
		pf("Evaluation failed: all %d rejections are ineligible.\n", cr.TotalRejections)

	default:
		pf("Score: %.0f%%\n", cr.Score())
		pf("Eligible rejections: %d\n", cr.EligibleRejections)
		pf("Eligible rejections preserved by this RTS: %d\n", cr.preserved())
	}
	p.Level--

	pf("Efficiency:\n")
	p.Level++
	if r.Efficiency.SampleDuration == 0 {
		pf("Evaluation failed: no test results with duration\n")
	} else {
		pf("Saved: %.0f%%\n", r.Efficiency.Score())
		pf("Test results analyzed: %d\n", r.Efficiency.TestResults)
		pf("Compute time in the sample: %s\n", r.Efficiency.SampleDuration)
		pf("Forecasted compute time: %s\n", r.Efficiency.ForecastDuration)
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

	printScore := func(score float64) {
		if math.IsNaN(score) {
			fmt.Fprintf(&r.buf, "?")
		} else {
			fmt.Fprintf(&r.buf, "%02.0f%%", score)
		}
	}

	fmt.Fprintf(&r.buf, "change recall: ")
	printScore(r.res.Safety.ChangeRecall.Score())
	fmt.Fprintf(&r.buf, " (%d data points)", r.res.Safety.ChangeRecall.EligibleRejections)

	fmt.Fprintf(&r.buf, " | efficiency: ")
	printScore(r.res.Efficiency.Score())
	fmt.Fprintf(&r.buf, " (%d data points)", r.res.Efficiency.TestResults)

	return r.buf.String()
}
