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
	"flag"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

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

	// The number of goroutines to spawn for each metric.
	// If <=0, defaults to 100.
	Concurrency int

	// TrainingSet are the change rejections to use for computing candidate
	// thresholds.
	TrainingSet *history.Player

	// EvalSet is the historical records to use for safety and efficiency
	// evaluation.
	EvalSet *history.Player

	// // How often to report progress. Defaults to 5s.
	// ProgressReportInterval time.Duration

	// If true, log lost rejections.
	// See also ChangeRecall.LostRejections.
	LogLostRejections bool
}

// RegisterFlags registers flags for the Eval fields.
func (e *Eval) RegisterFlags(fs *flag.FlagSet) error {
	fs.IntVar(&e.Concurrency, "j", defaultConcurrency, "Number of job to run parallel")
	fs.Var(&historyFileInputFlag{ptr: &e.TrainingSet}, "training-set", text.Doc(`
		Path to the history file for training.
		The distance data is ignored.
	`))
	fs.Var(&historyFileInputFlag{ptr: &e.EvalSet}, "eval-set", "Path to the history file for evaluation")
	//fs.DurationVar(&e.ProgressReportInterval, "progress-report-interval", defaultProgressReportInterval, "How often to report progress")
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
	res Result
	//buf                      bytes.Buffer
	// mostRecentProgressReport time.Time
	// mu                       sync.Mutex
}

func (r *evalRun) run(ctx context.Context) error {
	// Init internal state.
	r.res = Result{}

	logging.Infof(ctx, "training...")
	if err := r.train(ctx); err != nil {
		return errors.Annotate(err, "threshold computation failed").Err()
	}
	//r.mostRecentProgressReport = time.Time{}

	logging.Infof(ctx, "evaluating...")
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
		r.res.EvalRecords = r.EvalSet.TotalRecords()
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
		r.res.TrainingRecords = r.TrainingSet.TotalRecords()
		return errors.Annotate(err, "failed to playback history").Err()
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	// Build the distance/rank grid by combinding their percentiles.
	r.res.Thresholds = ThresholdGrid{}
	distancePercentiles := affectedness.quantiles(100, true)
	rankPercentiles := affectedness.quantiles(100, false)
	for row, distance := range distancePercentiles {
		for col, rank := range rankPercentiles {
			r.res.Thresholds[row][col].Value = Affectedness{Distance: distance.Distance, Rank: rank.Rank}
		}
	}
	return nil
}

// evaluateSafety reads rejections from r.rejectionC,
// updates r.res.Safety and calls r.maybeReportProgress.
func (r *evalRun) evaluateSafety(ctx context.Context) error {
	var rejectionBuckets, failureBuckets gridBuckets
	var totalRejections, totalFailures int32
	err := r.parallelize(ctx, func(ctx context.Context) error {
		return analyzeRejections(ctx, r.EvalSet.RejectionC, r.Strategy, func(ctx context.Context, rej analyzedRejection) error {
			rejectionBuckets.inc(&r.res.Thresholds, rej.Closest, 1)
			for _, af := range rej.Affectedness {
				failureBuckets.inc(&r.res.Thresholds, af, 1)
			}

			atomic.AddInt32(&totalRejections, 1)
			atomic.AddInt32(&totalFailures, int32(len(rej.Affectedness)))
			return nil
		})
	})
	if err != nil {
		return err
	}

	r.res.TotalRejections = int(totalRejections)
	r.res.TotalFailures = int(totalFailures)

	// Turn the "histogram" in the buckets into cumulative counts.
	rejectionBuckets.makeCumulative()
	failureBuckets.makeCumulative()
	for row := 0; row < 100; row++ {
		for col := 0; col < 100; col++ {
			r.res.Thresholds[row][col].PreservedRejections = int(rejectionBuckets[row][col])
			r.res.Thresholds[row][col].PreservedTestFailures = int(failureBuckets[row][col])
		}
	}
	return nil
}

// evaluateEfficiency reads test durations from r.durationC,
// updates r.res.Efficiency and calls r.maybeReportProgress.
func (r *evalRun) evaluateEfficiency(ctx context.Context) error {
	var durationBuckets gridBuckets
	var totalDuration int64
	err := r.parallelize(ctx, func(ctx context.Context) error {
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
			dur := int64(td.Duration.AsDuration())
			atomic.AddInt64(&totalDuration, dur)
			durationBuckets.inc(&r.res.Thresholds, out.TestVariantAffectedness[0], dur)
		}
		return ctx.Err()
	})
	if err != nil {
		return err
	}

	r.res.TotalDuration = time.Duration(totalDuration)

	// Turn the "histogram" in the buckets into cumulative durations.
	durationBuckets.makeCumulative()
	for row := 0; row < 100; row++ {
		for col := 0; col < 100; col++ {
			r.res.Thresholds[row][col].TestDuration = time.Duration(durationBuckets[row][col])
		}
	}
	return nil
}

// func (r *evalRun) maybeReportProgress(ctx context.Context) {
// 	interval := r.ProgressReportInterval
// 	if interval <= 0 {
// 		interval = defaultProgressReportInterval
// 	}

// 	now := clock.Now(ctx)
// 	switch {
// 	case r.mostRecentProgressReport.IsZero():
// 		r.mostRecentProgressReport = now

// 	case now.After(r.mostRecentProgressReport.Add(interval)):
// 		r.buf.Reset()
// 		r.res.oneLine(&r.buf)
// 		logging.Infof(ctx, "%s", r.buf.Bytes())
// 		r.mostRecentProgressReport = now
// 	}
// }

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

type gridBuckets [100][100]int64

func (b *gridBuckets) inc(g *ThresholdGrid, af Affectedness, delta int64) {
	for row := 0; row < 100; row++ {
		col := sort.Search(100, func(i int) bool {
			return g[row][i].Value.Rank >= af.Rank
		})
		if col < 100 {
			atomic.AddInt64(&b[row][col], delta)
		}
	}
}

func (b *gridBuckets) makeCumulative() {
	for row := 1; row < 100; row++ {
		for col := 1; col < 100; col++ {
			b[row][col] += b[row][col-1]
		}
	}
}
