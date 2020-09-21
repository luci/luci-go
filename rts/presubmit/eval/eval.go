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
)

// defaults
const (
	defaultWindowDays = 7
	defaultWorkers    = 100

	// defaultGerritQPSLimit is the default Gerrit QPS limit.
	// The default is chosen experimentally, with a goal to avoid hitting the
	// quota.
	defaultGerritQPSLimit = 10
)

// Input is input to an RTS Algorithm.
type Input struct {
	// ChangedFiles is a list of files changed in a CL.
	ChangedFiles []string `json:"changedFiles"`

	// The algorithm needs to decide whether to run this test.
	// For Chromium, TestID is a ResultDB TestID.
	Test *Test
}

// Output is the output of an RTS algorithm.
type Output struct {
	// ShouldRun is true if the test should run as a part of the suite.
	ShouldRun bool
}

// Algorithm accepts a list of changed files and returns a list of tests to
// run.
type Algorithm func(*Input) (*Output, error)

// RejectedPatchSet is a patchset rejected by CQ.
type RejectedPatchSet struct {
	Patchset  GerritPatchset `json:"patchset"`
	Timestamp time.Time      `json:"timestamp"`

	// FailedTests are the tests that caused the rejection.
	FailedTests []*Test `json:"failedTests"`
}

// Test describes a test.
type Test struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
}

// Eval estimates safety and precision of a given RTS algorithm.
type Eval struct {
	// The algorithm to evaluate.
	Algorithm Algorithm

	// The window of time to analyze, in days.
	// If <=0, defaults to 7.
	WindowsDays int

	// The number of goroutines to spawn.
	// If <=0, defaults to 100.
	Concurrency int

	// Directory where to cache fetched data.
	// If "", defaults to ${systemCacheDir}/chrome-rts.
	CacheDir string

	// Maximum QPS to send to Gerrit.
	// If <=0, defaults to 10.
	GerritQPSLimit int

	// RejectedPatchSetProvider retrieves all patchsets rejected by CQ within the
	// given date range, along with tests that caused the rejection.
	RejectedPatchSetProvider RejectedPatchSetProvider

	// TODO(nodir): potentially exclude patchsets where >1000 tests failed.
	// Most RTS algorithms would satisfy such a patchset.
	// // MaxFailedTests, if positive, is the maximum number of failed tests in a rejected patchset.
	// // Patchsets with more failed tests are considered noisy and are rejected.
	// // Defaults to 1000.
	// MaxFailedTests int
}

// RegisterFlags registers flags for the Eval fields.
func (e *Eval) RegisterFlags(fs *flag.FlagSet) {
	fs.IntVar(&e.WindowsDays, "window", defaultWindowDays, "The time window to analyze, in days")
	fs.IntVar(&e.Concurrency, "j", defaultWorkers, "Number of job to run parallel")
	fs.StringVar(&e.CacheDir, "cache-dir", "", "Path to the cache dir. Defaults to [CACHE_DIR]/chrome-rts")
	fs.IntVar(&e.GerritQPSLimit, "gerrit-qps-limit", defaultGerritQPSLimit, "Max Gerrit QPS")
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
	return &run.Result, nil
}
