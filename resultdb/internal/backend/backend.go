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

	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/server"
)

// ThrottledLogger emits a log item at most every MinHeartbeatInterval.
//
// It also holds a format string for the heartbeat message and a logging level.
//
// Its zero-value emits a debug-level log every time Beat() is called and
// includes all the args to the Beat function in the output.
type ThrottledLogger struct {
	MinHeartbeatInterval time.Duration
	Level                logging.Level
	Format               string
	lastHeartbeat        time.Time
	mut                  sync.Mutex
}

// Beat emits a log item if lastHeartbeat was longer than MinHeartbeatInterval
// ago.
//
// It uses a format string if set, otherwise it simply concatenates the args
// with fmt.Sprint().
func (tl ThrottledLogger) Beat(ctx context.Context, args ...interface{}) {
	start := clock.Now(ctx)
	tl.mut.Lock()
	if start.Sub(tl.lastHeartbeat) > tl.MinHeartbeatInterval {
		if tl.Format == "" {
			logging.Logf(ctx, tl.Level, fmt.Sprintf("Heartbeat: %s", fmt.Sprint(args...)))
		} else {
			logging.Logf(ctx, tl.Level, tl.Format, args...)
		}
		tl.lastHeartbeat = start
	}
	tl.mut.Unlock()
}

// processingLoop runs f repeatedly, until the context is cancelled.
// Ensures f is not called too often (minInterval) and backs off linearly
// on errors.
func processingLoop(ctx context.Context, activityName string, minInterval, maxSleep time.Duration, f func(context.Context) error) {
	attempt := 0
	var iterationCounter int64
	tl := ThrottledLogger{}
	tl.Format = fmt.Sprintf("%s has ran %%d iterations since start-up", activityName)
	tl.MinHeartbeatInterval = time.Duration(5) * time.Minute
	for ctx.Err() == nil {
		iterationCounter++
		tl.Beat(ctx, iterationCounter)
		start := clock.Now(ctx)
		if err := f(ctx); err == nil {
			attempt = 0
		} else {
			logging.Errorf(ctx, "Iteration failed: %s", err)

			attempt++
			sleep := time.Duration(attempt) * time.Second
			if sleep > maxSleep {
				sleep = maxSleep
			}
			time.Sleep(sleep)
		}
		if sleep := minInterval - clock.Since(ctx, start); sleep > 0 {
			time.Sleep(sleep)
		}
	}
	logging.Warningf(ctx, "Exiting loop for %s due to %v", activityName, ctx.Err())
}

// Options is backend server configuration.
type Options struct {
	// Whether to purge expired results.
	PurgeExiredResults bool
}

// InitServer initializes a backend server.
func InitServer(srv *server.Server, opts Options) {
	for _, taskType := range tasks.AllTypes {
		activity := fmt.Sprintf("resultdb.task.%s", taskType)
		srv.RunInBackground(activity, func(ctx context.Context) {
			runInvocationTasks(ctx, taskType)
		})
	}
	if opts.PurgeExiredResults {
		srv.RunInBackground("resultdb.purge_expired_results", purgeExpiredResults)
	}
}
