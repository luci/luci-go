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

package backend

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal/tasks"
)

// ThrottledLogger emits a log item at most every MinLogInterval.
//
// Its zero-value emits a debug-level log every time Log() is called.
type ThrottledLogger struct {
	MinLogInterval time.Duration
	Level          logging.Level
	lastLog        time.Time
	mu             sync.Mutex
}

// Log emits a log item at most once every MinLogInterval.
func (tl *ThrottledLogger) Log(ctx context.Context, format string, args ...interface{}) {
	now := clock.Now(ctx)
	tl.mu.Lock()
	if now.Sub(tl.lastLog) > tl.MinLogInterval {
		logging.Logf(ctx, tl.Level, format, args...)
		tl.lastLog = now
	}
	tl.mu.Unlock()
}

type backend struct {
	*Options
	bqExporter

	// taskWorkers is the number of goroutines that process invocation tasks.
	taskWorkers int
}

// cronGroup runs multiple cron jobs concurrently.
func (b *backend) cronGroup(ctx context.Context, replicas int, minInterval time.Duration, f func(ctx context.Context, replica int) error) {
	var wg sync.WaitGroup
	for i := 0; i < replicas; i++ {
		i := i
		ctx := logging.SetField(ctx, "cron_replica", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.cron(ctx, minInterval, func(ctx context.Context) error {
				return f(ctx, i)
			})
		}()
	}
	wg.Wait()
}

// cron runs f repeatedly, until the context is cancelled.
//
// Ensures f is not called too often (minInterval).
// minInterval is ignored if b.ForceCronInterval is >0.
func (b *backend) cron(ctx context.Context, minInterval time.Duration, f func(context.Context) error) {
	defer logging.Warningf(ctx, "Exiting cron")

	// call calls f with a timeout and catches a panic.
	call := func(ctx context.Context) error {
		defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
			logging.Errorf(ctx, "Caught panic: %s\n%s", p.Reason, p.Stack)
		})
		return f(ctx)
	}

	if b.ForceCronInterval > 0 {
		minInterval = b.ForceCronInterval
	}

	var iterationCounter int
	tl := ThrottledLogger{MinLogInterval: 5 * time.Minute}
	for {
		iterationCounter++
		tl.Log(ctx, "%d iterations have run since start-up", iterationCounter)
		start := clock.Now(ctx)
		if err := call(ctx); err != nil {
			logging.Errorf(ctx, "Iteration failed: %s", err)
		}

		// Ensure minInterval between iterations.
		if sleep := minInterval - clock.Since(ctx, start); sleep > 0 {
			// Add jitter: +-10% of sleep time to desynchronize cron jobs.
			sleep = sleep - sleep/10 + time.Duration(mathrand.Intn(ctx, int(sleep/5)))
			select {
			case <-time.After(sleep):
			case <-ctx.Done():
				return
			}
		}
	}
}

// Options is backend server configuration.
type Options struct {
	// PurgeExpiredResults instructs backend to purge expired results.
	PurgeExpiredResults bool

	// ForceCronInterval forces minimum interval in cron jobs.
	// Useful in integration tests to reduce the test time.
	ForceCronInterval time.Duration

	// ForceLeaseDuration is the duration to use instead of task-type-specific
	// durations, if ForceLeaseDuration > 0.
	// Useful in integration tests to reduce the test time.
	ForceLeaseDuration time.Duration
}

// InitServer initializes a backend server.
func InitServer(srv *server.Server, opts Options) {
	b := &backend{
		Options:     &opts,
		taskWorkers: 100,
		bqExporter: bqExporter{
			// TODO(nodir): move all these constants to Options and bind them to flags.

			maxBatchRowCount: 500,
			// HTTP request size limit is 10 MiB according to
			// https://cloud.google.com/bigquery/quotas#streaming_inserts
			// Use a smaller size as the limit since we are only using the size of
			// test results to estimate the whole payload size.
			maxBatchSize: 6e6,
			putLimiter:   rate.NewLimiter(100, 1),

			// 1 batch is ~6Mb (see above).
			// Allow ~2Gb => 2Gb/6Mb = 333 batches
			batchSem: semaphore.NewWeighted(300),
		},
	}

	for _, taskType := range tasks.AllTypes {
		taskType := taskType
		activity := fmt.Sprintf("resultdb.task.%s", taskType)
		srv.RunInBackground(activity, func(ctx context.Context) {
			b.runInvocationTasks(ctx, taskType)
		})
	}
	if opts.PurgeExpiredResults {
		srv.RunInBackground("resultdb.purge_expired_results", b.purgeExpiredResults)
	}
}
