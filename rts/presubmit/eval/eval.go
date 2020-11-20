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

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/presubmit/eval/history"
)

// defaults
const (
	defaultConcurrency = 100

	// defaultGerritQPSLimit is the default Gerrit QPS limit.
	// The default is chosen experimentally, with a goal to avoid hitting the
	// short-term quota.
	defaultGerritQPSLimit = 10

	defaultProgressReportInterval = 5 * time.Second
)

// Eval estimates safety and efficiency of a given RTS algorithm.
type Eval struct {
	// The algorithm to evaluate.
	Algorithm Algorithm

	// The number of goroutines to spawn for each metric.
	// If <=0, defaults to 100.
	Concurrency int

	// Directory where to cache fetched data.
	// If "", defaults to ${systemCacheDir}/chrome-rts.
	CacheDir string

	// Maximum QPS to send to Gerrit.
	// If <=0, defaults to 10.
	GerritQPSLimit int

	// Historical records to use for evaluation.
	History *history.Reader

	// How often to report progress. Defaults to 5s.
	ProgressReportInterval time.Duration

	// If true, log eligible lost rejections.
	// See also ChangeRecall.LostRejections.
	LogLostRejections bool
}

// RegisterFlags registers flags for the Eval fields.
func (e *Eval) RegisterFlags(fs *flag.FlagSet) error {
	fs.IntVar(&e.Concurrency, "j", defaultConcurrency, "Number of job to run parallel")

	cacheDir, err := defaultCacheDir()
	if err != nil {
		return err
	}

	fs.StringVar(&e.CacheDir, "cache-dir", cacheDir, "Path to the cache dir")
	fs.IntVar(&e.GerritQPSLimit, "gerrit-qps-limit", defaultGerritQPSLimit, "Max Gerrit QPS")
	fs.Var(&historyFileInputFlag{ptr: &e.History}, "history", "Path to the history file")
	fs.DurationVar(&e.ProgressReportInterval, "progress-report-interval", defaultProgressReportInterval, "How often to report progress")
	fs.BoolVar(&e.LogLostRejections, "log-lost-rejections", false, "Log every lost rejection, to diagnose the RTS algorithm")
	return nil
}

// ValidateFlags validates values of flags registered using RegisterFlags.
func (e *Eval) ValidateFlags() error {
	if e.History == nil {
		return errors.New("-history is required")
	}
	return nil
}

// Run evaluates the algorithm.
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
