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

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"

	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal/cron"
	"go.chromium.org/luci/resultdb/internal/tasks"
)

type backend struct {
	*Options
	bqExporter
}

func (b *backend) cron(ctx context.Context, minInterval time.Duration, f func(ctx context.Context) error) {
	if b.ForceCronInterval > 0 {
		minInterval = b.ForceCronInterval
	}

	cron.Run(ctx, minInterval, f)
}

// Options is backend server configuration.
type Options struct {
	// ForceCronInterval forces minimum interval in cron jobs.
	// Useful in integration tests to reduce the test time.
	ForceCronInterval time.Duration

	// ForceLeaseDuration is the duration to use instead of task-type-specific
	// durations, if ForceLeaseDuration > 0.
	// Useful in integration tests to reduce the test time.
	ForceLeaseDuration time.Duration

	// TaskWorkers is the number of goroutines that process invocation tasks.
	TaskWorkers int

	UseInsertIDs bool
}

// InitServer initializes a backend server.
func InitServer(srv *server.Server, opts Options) {
	b := &backend{
		Options: &opts,
		bqExporter: bqExporter{
			useInsertIDs: opts.UseInsertIDs,

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
}
