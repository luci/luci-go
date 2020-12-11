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
	"context"
	"math"
	"time"

	"golang.org/x/sync/errgroup"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

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

// Score returns the efficiency score.
// May return NaN.
func (e *Efficiency) Score() float64 {
	if e.SampleDuration == 0 {
		return math.NaN()
	}
	saved := e.SampleDuration - e.ForecastDuration
	return float64(saved) / float64(e.SampleDuration)
}

// evaluateEfficiency reads test durations from r.durationC,
// updates r.res.Efficiency and calls r.maybeReportProgress.
func (r *evalRun) evaluateEfficiency(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	// Run the selection strategy in r.Concurrency goroutines.
	for i := 0; i < r.Concurrency; i++ {
		eg.Go(func() error {
			in := Input{TestVariants: make([]*evalpb.TestVariant, 1)}
			out := &Output{TestVariantAffectedness: make(AffectednessSlice, 1)}
			for td := range r.History.DurationC {
				// Invoke the strategy.
				in.ChangedFiles = in.ChangedFiles[:0]
				in.ensureChangedFilesInclude(td.Patchsets...)
				in.TestVariants[0] = td.TestVariant
				out.TestVariantAffectedness[0] = Affectedness{}
				if err := r.Strategy(ctx, in, out); err != nil {
					return err
				}

				// Record results.
				dur := td.Duration.AsDuration()
				r.mu.Lock()
				r.res.Efficiency.TestResults++
				r.res.Efficiency.SampleDuration += dur
				if r.shouldRun(out.TestVariantAffectedness[0]) {
					r.res.Efficiency.ForecastDuration += dur
				}
				r.maybeReportProgress(ctx)
				r.mu.Unlock()
			}
			return ctx.Err()
		})
	}

	return eg.Wait()
}
