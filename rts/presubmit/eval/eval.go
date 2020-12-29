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

const defaultConcurrency = 100

// Eval estimates safety and efficiency of a given selection strategy.
type Eval struct {
	// The selection strategy to evaluate.
	Strategy Strategy

	// The number of goroutines to spawn for each metric.
	// If <=0, defaults to 100.
	Concurrency int

	// Rejections are used to evaluate safety of the strategy.
	Rejections *history.Player

	// Durations are used to evaluate efficiency of the strategy.
	Durations *history.Player

	// LogFurthest instructs to log rejections for which failed tests have large
	// distance, as concluded by the selection strategy.
	// LogFurthest is the number of rejections to print, ordered by descending
	// distance.
	// This can help diagnosing the selection strategy.
	//
	// TODO(nodir): implement this.
	LogFurthest int
}

// RegisterFlags registers flags for the Eval fields.
func (e *Eval) RegisterFlags(fs *flag.FlagSet) error {
	fs.IntVar(&e.Concurrency, "j", defaultConcurrency, "Number of job to run parallel")
	fs.Var(&historyFileInputFlag{ptr: &e.Rejections}, "rejections", text.Doc(`
		Path to the history file with change rejections. Used for safety evaluation.
	`))
	fs.Var(&historyFileInputFlag{ptr: &e.Durations}, "durations", text.Doc(`
		Path to the history file with test durations. Used for efficiency evaluation.
	`))
	fs.IntVar(&e.LogFurthest, "log-furthest", 0, text.Doc(`
		Log rejections for which failed tests have large distance,
		as concluded by the selection strategy.
		The flag value is the number of rejections to print, ordered by descending
		distance.
		This can help diagnosing the selection strategy.
	`))
	return nil
}

// ValidateFlags validates values of flags registered using RegisterFlags.
func (e *Eval) ValidateFlags() error {
	if e.Rejections == nil {
		return errors.New("-rejections is required")
	}
	if e.Durations == nil {
		return errors.New("-durations is required")
	}
	return nil
}

// Run evaluates the candidate strategy.
func (e *Eval) Run(ctx context.Context) (*Result, error) {
	res := &Result{}

	logging.Infof(ctx, "evaluating safety...")
	grid, err := e.evaluateSafety(ctx, res)
	if err != nil {
		return nil, errors.Annotate(err, "failed to evaluate safety").Err()
	}

	logging.Infof(ctx, "evaluating efficiency...")
	if err := e.evaluateEfficiency(ctx, res, grid); err != nil {
		return nil, errors.Annotate(err, "failed to evaluate efficiency").Err()
	}

	res.Thresholds = grid.Slice()
	return res, nil
}

// evaluateSafety computes thresholds and total/preserved
// rejections/testFailures.
func (e *Eval) evaluateSafety(ctx context.Context, res *Result) (*thresholdGrid, error) {
	var changeAffectedness AffectednessSlice
	var testAffectedness AffectednessSlice
	var mu sync.Mutex

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	// Play back the history.
	eg.Go(func() error {
		err := e.Rejections.PlaybackRejections(ctx)
		return errors.Annotate(err, "failed to playback history").Err()
	})

	e.goMany(eg, func() error {
		for rej := range e.Rejections.RejectionC {
			// TODO(crbug.com/1112125): skip the patchset if it has a ton of failed tests.
			// Most selection strategies would reject such a patchset, so it represents noise.

			// Invoke the strategy.
			in := Input{TestVariants: rej.FailedTestVariants}
			in.ensureChangedFilesInclude(rej.Patchsets...)
			out := &Output{TestVariantAffectedness: make(AffectednessSlice, len(in.TestVariants))}
			if err := e.Strategy(ctx, in, out); err != nil {
				return errors.Annotate(err, "the selection strategy failed").Err()
			}

			// The affectedness of a change is based on the most affected failed test.
			closest, err := out.TestVariantAffectedness.closest()
			if err != nil {
				return err
			}

			mu.Lock()
			changeAffectedness = append(changeAffectedness, out.TestVariantAffectedness[closest])
			testAffectedness = append(testAffectedness, out.TestVariantAffectedness...)
			mu.Unlock()
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	if len(changeAffectedness) == 0 {
		return nil, errors.New("no change rejections")
	}

	// Initialize the grid.
	// Compute distance/rank thresholds by taking distance/rank percentiles in
	// changeAffectedness. Row/column indexes represent ChangeRecall scores.
	grid := &thresholdGrid{}
	distancePercentiles, rankPercentiles := grid.init(changeAffectedness)
	logging.Infof(ctx, "Distance percentiles: %v", distancePercentiles)
	logging.Infof(ctx, "Rank percentiles: %v", rankPercentiles)

	// Evaluate safety of *combinations* of thresholds.

	losses := func(afs AffectednessSlice) *bucketGrid {
		var buckets bucketGrid
		for _, af := range afs {
			buckets.inc(grid, af, 1)
		}
		buckets.makeCumulative()
		return &buckets
	}

	res.TotalRejections = len(changeAffectedness)
	lostRejections := losses(changeAffectedness)

	res.TotalTestFailures = len(testAffectedness)
	lostFailures := losses(testAffectedness)

	for row := 0; row < 100; row++ {
		for col := 0; col < 100; col++ {
			t := &grid[row][col]
			t.PreservedRejections = res.TotalRejections - int(lostRejections[row+1][col+1])
			t.PreservedTestFailures = res.TotalTestFailures - int(lostFailures[row+1][col+1])
			t.ChangeRecall = float64(t.PreservedRejections) / float64(res.TotalRejections)
			t.DistanceChangeRecall = float64(row+1) / 100
			t.RankChangeRecall = float64(col+1) / 100
			t.TestRecall = float64(t.PreservedTestFailures) / float64(res.TotalTestFailures)
		}
	}
	return grid, nil
}

// evaluateEfficiency computes total and saved durations.
func (e *Eval) evaluateEfficiency(ctx context.Context, res *Result, grid *thresholdGrid) error {
	// Process test durations in parallel and increment appropriate counters.
	var savedDurations bucketGrid
	var totalDuration int64
	var dataPoints int64

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	// Play back the history.
	eg.Go(func() error {
		err := e.Durations.PlaybackDurations(ctx)
		return errors.Annotate(err, "failed to playback history").Err()
	})

	e.goMany(eg, func() error {
		in := Input{TestVariants: make([]*evalpb.TestVariant, 1)}
		out := &Output{TestVariantAffectedness: make(AffectednessSlice, 1)}
		for td := range e.Durations.DurationC {
			// Invoke the strategy.
			in.ChangedFiles = in.ChangedFiles[:0]
			in.ensureChangedFilesInclude(td.Patchsets...)
			in.TestVariants[0] = td.TestVariant
			out.TestVariantAffectedness[0] = Affectedness{}
			if err := e.Strategy(ctx, in, out); err != nil {
				return errors.Annotate(err, "the selection strategy failed").Err()
			}

			// Record results.
			dur := int64(td.Duration.AsDuration())
			atomic.AddInt64(&totalDuration, dur)
			savedDurations.inc(grid, out.TestVariantAffectedness[0], dur)

			if curCount := atomic.AddInt64(&dataPoints, 1); curCount%1000 == 0 {
				logging.Infof(ctx, "processed %d durations", curCount)
			}
		}
		return ctx.Err()
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	if totalDuration == 0 {
		return errors.New("sum of test durations is 0")
	}

	// Incroporate the counters into dest.

	res.TotalDuration = time.Duration(totalDuration)
	savedDurations.makeCumulative()
	for row := 0; row < 100; row++ {
		for col := 0; col < 100; col++ {
			t := &grid[row][col]
			t.SavedDuration = time.Duration(savedDurations[row+1][col+1])
			t.Savings = float64(t.SavedDuration) / float64(res.TotalDuration)
		}
	}
	return nil
}

func (e *Eval) goMany(eg *errgroup.Group, f func() error) {
	concurrency := e.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	for i := 0; i < concurrency; i++ {
		eg.Go(func() error {
			return f()
		})
	}
}

// bucketGrid is an auxulary data structure to compute cumulative counters
// in ThresholdGrid. Each cell contains the number of data points lost by that
// bucket.
//
// bucketGrid is used in two phases:
//   1) For each data point, call inc().
//   2) Call makeCumulative() and incorporate bucketGrid into ThresholdGrid.
//
// The structure of bucketGrid is similar to ThresholdGrid, except bucketGrid
// cell (R, C) corresponds to ThresholdGrid cell (R-1, C-1). This is because the
// bucketGrid is padded with extra row 0 and column 0 for data points that were
// not lost by any distance or by any rank.
type bucketGrid [101][101]int64

// inc increments the counter in the cell (R, C) where row R has the largest
// distance less than af.Distance, and C has the largest rank less than af.Rank.
// In other words, it increments the largest thresholds that missed the data
// point.
//
// Goroutine-safe.
func (b *bucketGrid) inc(g *thresholdGrid, af Affectedness, delta int64) {
	row := sort.Search(100, func(i int) bool {
		return g[i][0].Value.Distance >= af.Distance
	})
	col := sort.Search(100, func(i int) bool {
		return g[0][i].Value.Rank >= af.Rank
	})

	// For each of distance and rank, we have found the index of the smallest
	// threshold satisfied by af.
	// We need the *largest* threshold *not* satisfied by af, i.e. the preceding
	// index. Indexes in bucketGrid are already shifted by one, so use row and
	// col as is.
	atomic.AddInt64(&b[row][col], delta)
}

// makeCumulative makes all counters cumulative.
// Not idempotent.
func (b *bucketGrid) makeCumulative() {
	for row := 99; row >= 0; row-- {
		for col := 99; col >= 0; col-- {
			b[row][col] += b[row][col+1]
		}
	}
	for col := 99; col >= 0; col-- {
		for row := 99; row >= 0; row-- {
			b[row][col] += b[row+1][col]
		}
	}
}
