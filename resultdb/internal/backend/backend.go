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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/server"
)

// processingLoop runs f repeatedly, until the context is cancelled.
// Ensures f is not called too often (minInterval) and backs off linearly
// on errors.
func processingLoop(ctx context.Context, minInterval, maxSleep time.Duration, f func(context.Context) error) {
	attempt := 0
	for ctx.Err() == nil {
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
