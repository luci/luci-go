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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"

	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/server"
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

// processingLoop runs f repeatedly, until the context is cancelled.
// Ensures f is not called too often and backs off linearly on errors.
func processingLoop(ctx context.Context, minInterval, maxSleep, iterationTimeout time.Duration, f func(context.Context) error) {
	defer func() {
		if ctx.Err() != nil {
			logging.Warningf(ctx, "Exiting loop due to %v", ctx.Err())
		}
	}()

	// call calls f with a timeout and catches a panic.
	call := func(ctx context.Context) error {
		defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
			logging.Errorf(ctx, "Caught panic: %s\n%s", p.Reason, p.Stack)
		})
		ctx, _ = context.WithTimeout(ctx, iterationTimeout)
		return f(ctx)
	}

	attempt := 0
	var iterationCounter int64
	tl := ThrottledLogger{MinLogInterval: 5 * time.Minute}
	for {
		iterationCounter++
		tl.Log(ctx, "%d iterations have run since start-up", iterationCounter)
		start := clock.Now(ctx)
		sleep := time.Duration(0)
		if err := call(ctx); err == nil {
			attempt = 0
		} else {
			logging.Errorf(ctx, "Iteration failed: %s", err)

			attempt++
			sleep = time.Duration(attempt) * time.Second
		}

		if minSleep := minInterval - clock.Since(ctx, start); sleep < minSleep {
			sleep = minSleep
		}
		if sleep > maxSleep {
			sleep = maxSleep
		}

		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return
		}
	}
}

// Options is backend server configuration.
type Options struct {
	// Whether to purge expired results.
	PurgeExpiredResults bool
}

// InitServer initializes a backend server.
func InitServer(srv *server.Server, opts Options) {
	for _, taskType := range tasks.AllTypes {
		taskType := taskType
		activity := fmt.Sprintf("resultdb.task.%s", taskType)
		srv.RunInBackground(activity, func(ctx context.Context) {
			runInvocationTasks(ctx, taskType)
		})
	}
	if opts.PurgeExpiredResults {
		srv.RunInBackground("resultdb.purge_expired_results", purgeExpiredResults)
	}
}
