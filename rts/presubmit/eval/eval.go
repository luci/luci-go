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
	"flag"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/rts/presubmit/eval/history"
	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// defaults
const (
	defaultConcurrency = 100

	defaultProgressReportInterval = 5 * time.Second
)

// Eval estimates safety and efficiency of a given selection strategy.
type Eval struct {
	// The selection strategy to evaluate.
	Strategy Strategy

	// MaxDistance is the maximum distance to run a test.
	// If a test's distance is <= MaxDistance, then it is executed, regardless of
	// MaxRank.
	//
	// TODO(nodir): delete this.
	MaxDistance float64

	// MaxRank is a test rank threshold: if a test's rank <= MaxRank, then it is
	// executed, regardless of MaxDistance.
	// Note that multiple tests may have the same rank.
	//
	// TODO(nodir): delete this.
	MaxRank int

	// The number of goroutines to spawn for each metric.
	// If <=0, defaults to 100.
	Concurrency int

	// TrainingSet are the change rejections to use for computing candidate
	// thresholds.
	TrainingSet *history.Player

	// EvalSet is the historical records to use for safety and efficiency
	// evaluation.
	EvalSet *history.Player

	// How often to report progress. Defaults to 5s.
	ProgressReportInterval time.Duration

	// If true, log lost rejections.
	// See also ChangeRecall.LostRejections.
	LogLostRejections bool
}

// RegisterFlags registers flags for the Eval fields.
func (e *Eval) RegisterFlags(fs *flag.FlagSet) error {
	// The default value of 0.5 makes sense for those selection strategies that
	// use distance between 0.0 and 1.0.
	fs.Float64Var(&e.MaxDistance, "max-distance", 0.5, text.Doc(`
		Max distance from tests to the changed files.
		If a test is closer than or equal -max-distance, then it is selected regardless of -max-rank.
	`))
	fs.IntVar(&e.MaxRank, "max-rank", 0, text.Doc(`
		Max rank for tests to run.
		If a test has a rank less or equal -max-rank, then it is selected regardless of -max-distance.
		Ignored if 0
	`))
	fs.IntVar(&e.Concurrency, "j", defaultConcurrency, "Number of job to run parallel")
	fs.Var(&historyFileInputFlag{ptr: &e.TrainingSet}, "training-set", text.Doc(`
		Path to the history file for training.
		The distance data is ignored.
	`))
	fs.Var(&historyFileInputFlag{ptr: &e.EvalSet}, "eval-set", "Path to the history file for evaluation")
	fs.DurationVar(&e.ProgressReportInterval, "progress-report-interval", defaultProgressReportInterval, "How often to report progress")
	fs.BoolVar(&e.LogLostRejections, "log-lost-rejections", false, "Log every lost rejection, to diagnose the selection strategy")
	return nil
}

// ValidateFlags validates values of flags registered using RegisterFlags.
func (e *Eval) ValidateFlags() error {
	if e.TrainingSet == nil {
		return errors.New("-training-set is required")
	}
	if e.EvalSet == nil {
		return errors.New("-eval-set is required")
	}
	return nil
}

// Run evaluates the candidate strategy.
func (e *Eval) Run(ctx context.Context) (*Result, error) {
	run := evalRun{
		// make a copy of settings
		Eval: *e,
	}
	if err := run.run(ctx); err != nil {
		return nil, err
	}
	return &run.res, nil
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

	logging.Infof(ctx, "training...")
	if err := r.train(ctx); err != nil {
		return errors.Annotate(err, "threshold computation failed").Err()
	}

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
		err := r.EvalSet.Playback(ctx)
		r.res.TotalRecords = r.EvalSet.TotalRecords()
		return errors.Annotate(err, "failed to playback history").Err()
	})

	return eg.Wait()
}

// train computes r.res.Thresholds.
func (r *evalRun) train(ctx context.Context) error {
	var affectedness AffectednessSlice
	var mu sync.Mutex

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()
	eg.Go(func() error {
		return r.parallelize(ctx, func(ctx context.Context) error {
			return analyzeRejections(ctx, r.TrainingSet.RejectionC, r.Strategy, func(ctx context.Context, rej analyzedRejection) error {
				mu.Lock()
				affectedness = append(affectedness, rej.Closest)
				mu.Unlock()
				return nil
			})
		})
	})

	eg.Go(func() error {
		err := r.TrainingSet.PlaybackIgnoreDurations(ctx)
		return errors.Annotate(err, "failed to playback history").Err()
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	// Populate the grid.
	// Compute distance/rank thresholds by taking distance/rank percentiles.
	distancePercentiles, rankPercentiles := r.res.Thresholds.init(affectedness)
	logging.Infof(ctx, "Distance percentiles: %v", distancePercentiles)
	logging.Infof(ctx, "Rank percentiles: %v", rankPercentiles)
	return nil
}

// evaluateSafety reads rejections from r.rejectionC,
// updates r.res.Safety and calls r.maybeReportProgress.
func (r *evalRun) evaluateSafety(ctx context.Context) error {
	return r.parallelize(ctx, func(ctx context.Context) error {
		cr := &r.res.Safety.ChangeRecall
		tr := &r.res.Safety.TestRecall

		buf := &bytes.Buffer{}
		p := &rejectionPrinter{printer: newPrinter(buf)}
		return analyzeRejections(ctx, r.EvalSet.RejectionC, r.Strategy, func(ctx context.Context, rej analyzedRejection) error {
			// The rejection is preserved if at least one test ran.
			// Conclude that based on the closest test.
			preservedRejection := r.shouldRun(rej.Closest)

			if !preservedRejection && r.LogLostRejections {
				buf.Reset()
				p.rejection(rej.Rejection)
				logging.Infof(ctx, "%s", buf.Bytes())
			}

			r.mu.Lock()
			defer r.mu.Unlock()
			cr.TotalRejections++
			tr.TotalFailures += len(rej.FailedTestVariants)
			if !preservedRejection {
				cr.LostRejections = append(cr.LostRejections, rej.Rejection)
			}
			for i, af := range rej.Affectedness {
				if !r.shouldRun(af) {
					tr.LostFailures = append(tr.LostFailures, LostTestFailure{
						Rejection:   rej.Rejection,
						TestVariant: rej.FailedTestVariants[i],
					})
				}
			}

			r.maybeReportProgress(ctx)
			return nil
		})
	})
}

// evaluateEfficiency reads test durations from r.durationC,
// updates r.res.Efficiency and calls r.maybeReportProgress.
func (r *evalRun) evaluateEfficiency(ctx context.Context) error {
	return r.parallelize(ctx, func(ctx context.Context) error {
		in := Input{TestVariants: make([]*evalpb.TestVariant, 1)}
		out := &Output{TestVariantAffectedness: make(AffectednessSlice, 1)}
		for td := range r.EvalSet.DurationC {
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
		r.buf.Reset()
		r.res.oneLine(&r.buf)
		logging.Infof(ctx, "%s", r.buf.Bytes())
		r.mostRecentProgressReport = now
	}
}

func (e *Eval) shouldRun(a Affectedness) bool {
	return a.Distance <= e.MaxDistance || (a.Rank != 0 && a.Rank <= e.MaxRank)
}

func (e *Eval) parallelize(ctx context.Context, f func(ctx context.Context) error) error {
	eg, ctx := errgroup.WithContext(ctx)
	concurrency := e.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	for i := 0; i < concurrency; i++ {
		eg.Go(func() error {
			return f(ctx)
		})
	}
	return eg.Wait()
}
