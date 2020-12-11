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
	"time"

	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/presubmit/eval/history"
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
	// TODO(nodir): automatically compute the optimal MaxDistance based on
	// MinChangeRecall and MinTestRecall.
	MaxDistance float64

	// MaxRank is a test rank threshold: if a test's rank <= MaxRank, then it is
	// executed, regardless of MaxDistance.
	// Note that multiple tests may have the same rank.
	//
	// TODO(nodir): automatically compute the optimal MaxRank based on
	// MinChangeRecall and MinTestRecall.
	MaxRank int

	// The number of goroutines to spawn for each metric.
	// If <=0, defaults to 100.
	Concurrency int

	// Historical records to use for evaluation.
	History *history.Reader

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
	fs.Var(&historyFileInputFlag{ptr: &e.History}, "history", "Path to the history file")
	fs.DurationVar(&e.ProgressReportInterval, "progress-report-interval", defaultProgressReportInterval, "How often to report progress")
	fs.BoolVar(&e.LogLostRejections, "log-lost-rejections", false, "Log every lost rejection, to diagnose the selection strategy")
	return nil
}

// ValidateFlags validates values of flags registered using RegisterFlags.
func (e *Eval) ValidateFlags() error {
	if e.History == nil {
		return errors.New("-history is required")
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

type historyFileInputFlag struct {
	path string
	ptr  **history.Reader
}

func (f *historyFileInputFlag) Set(val string) error {
	r, err := history.OpenFile(val)
	if err != nil {
		return err
	}

	f.path = val
	*f.ptr = r
	return nil
}

func (f *historyFileInputFlag) String() string {
	return f.path
}

func (e *Eval) shouldRun(a Affectedness) bool {
	return a.Distance <= e.MaxDistance || (a.Rank != 0 && a.Rank <= e.MaxRank)
}
