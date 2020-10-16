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

	progress progress

	auth   *auth.Authenticator
	gerrit *gerritClient
}

func (r *evalRun) run(ctx context.Context) (*Result, error) {
	if err := r.Init(ctx); err != nil {
		return nil, err
	}

	ret := &Result{}

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	// Analyze safety.
	rejectionC := make(chan *evalpb.Rejection)
	eg.Go(func() error {
		safety, err := r.evaluateSafety(ctx, rejectionC)
		if err != nil {
			return errors.Annotate(err, "failed to evaluate safety").Err()
		}
		ret.Safety = *safety
		return nil
	})

	// Analyze efficiency.
	durationC := make(chan *evalpb.TestDuration)
	eg.Go(func() error {
		efficiency, err := r.evaluateEfficiency(ctx, durationC)
		if err != nil {
			return errors.Annotate(err, "failed to evaluate efficiency").Err()
		}
		ret.Efficiency = *efficiency
		return nil
	})

	// Play back the history.
	eg.Go(func() error {
		defer func() {
			close(rejectionC)
			close(durationC)
			r.History.Close()
		}()

		for {
			rec, err := r.History.Read()
			switch {
			case err == io.EOF:
				return nil
			case err != nil:
				return errors.Annotate(err, "failed to read history").Err()
			}

			ret.TotalRecords++

			// Send the record to the appropriate channel.
			switch data := rec.Data.(type) {
			case *evalpb.Record_Rejection:
				select {
				case <-ctx.Done():
					return ctx.Err()
				case rejectionC <- data.Rejection:
				}
			case *evalpb.Record_TestDuration:
				select {
				case <-ctx.Done():
					return ctx.Err()
				case durationC <- data.TestDuration:
				}
			default:
				panic(fmt.Sprintf("unexpected record %s", rec))
			}
		}
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return ret, nil
}

// Print prints the results to w.
func (r *Result) Print(w io.Writer) error {
	p := newPrinter(w)
	pf := p.printf

	pf("Safety:\n")
	p.Level++
	switch {
	case r.Safety.TotalRejections == 0:
		pf("Evaluation failed: rejections not found\n")

	case r.Safety.EligibleRejections == 0:
		pf("Evaluation failed: all %d rejections are ineligible.\n", r.Safety.TotalRejections)

	default:
		pf("Score: %.0f%%\n", r.Safety.Score())
		pf("# of eligible rejections: %d\n", r.Safety.EligibleRejections)
		pf("# of them preserved by this RTS: %d\n", r.Safety.preserved())
	}
	p.Level--

	pf("Efficiency:\n")
	p.Level++
	if r.Efficiency.SampleDuration == 0 {
		pf("Evaluation failed: no test results with duration\n")
	} else {
		pf("Saved: %.0f%%\n", r.Efficiency.Score())
		pf("Compute time in the sample: %s\n", r.Efficiency.SampleDuration)
		pf("Forecasted compute time: %s\n", r.Efficiency.ForecastDuration)
	}
	p.Level--

	pf("Total records: %d\n", r.TotalRecords)
	return p.err
}

// progress captures the most recent state of evaluation and occasionally
// logs it.
//
// It is goroutine-safe.
type progress struct {
	reportInterval     time.Duration
	recentResult       Result
	loggedMostRecently time.Time
	mu                 sync.Mutex

	buf bytes.Buffer
}

func (p *progress) UpdateCurrentSafety(ctx context.Context, s Safety) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.recentResult.Safety = s
	p.maybeLog(ctx)
}

func (p *progress) UpdateCurrentEfficiency(ctx context.Context, e Efficiency) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.recentResult.Efficiency = e
	p.maybeLog(ctx)
}

func (p *progress) maybeLog(ctx context.Context) {
	interval := p.reportInterval
	if interval <= 0 {
		interval = defaultProgressReportInterval
	}

	now := clock.Now(ctx)
	switch {
	case p.loggedMostRecently.IsZero():
		p.loggedMostRecently = now

	case now.After(p.loggedMostRecently.Add(interval)):
		logging.Infof(ctx, "%s", p.reportLine())
		p.loggedMostRecently = now
	}
}

func (p *progress) reportLine() string {
	p.buf.Reset()

	printScore := func(score float64) {
		if math.IsNaN(score) {
			fmt.Fprintf(&p.buf, "?")
		} else {
			fmt.Fprintf(&p.buf, "%02.0f%%", score)
		}
	}

	fmt.Fprintf(&p.buf, "safety: ")
	printScore(p.recentResult.Safety.Score())

	fmt.Fprintf(&p.buf, " | efficiency: ")
	printScore(p.recentResult.Efficiency.Score())

	return p.buf.String()
}
