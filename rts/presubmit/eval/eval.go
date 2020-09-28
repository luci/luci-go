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
)

// defaults
const (
	defaultWindowDays  = 7
	defaultConcurrency = 100

	// defaultGerritQPSLimit is the default Gerrit QPS limit.
	// The default is chosen experimentally, with a goal to avoid hitting the
	// short-term quota.
	defaultGerritQPSLimit = 10
)

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

	// The evaluation backend to use.
	Backend Backend
}

// RegisterFlags registers flags for the Eval fields.
func (e *Eval) RegisterFlags(fs *flag.FlagSet) error {
	fs.IntVar(&e.WindowsDays, "window", defaultWindowDays, "The time window to analyze, in days")
	fs.IntVar(&e.Concurrency, "j", defaultConcurrency, "Number of job to run parallel")

	cacheDir, err := defaultCacheDir()
	if err != nil {
		return err
	}

	fs.StringVar(&e.CacheDir, "cache-dir", cacheDir, "Path to the cache dir")
	fs.IntVar(&e.GerritQPSLimit, "gerrit-qps-limit", defaultGerritQPSLimit, "Max Gerrit QPS")
	return nil
}

// Run evaluates the algorithm.
func (e *Eval) Run(ctx context.Context) (*Result, error) {
	run := evalRun{
		// make a copy of settings
		Eval: *e,
	}
	return run.run(ctx)
}
