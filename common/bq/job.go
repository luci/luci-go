// Copyright 2024 The LUCI Authors.
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

package bq

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// WaitForJob waits for a BigQuery job to finish.
// If after timeout and the job has not finished, it will attempt
// to cancel the job. The cancellation is based on best-effort,
// so if there is an error, we just log instead of throwing the error.
// This is to avoid jobs overrunning each other and triggering
// a death spiral of write contention / starving each other of resources.
// The actual timeout for bigquery job will be context timeout reduced
// by 5 seconds. It is for the cancelling job to execute.
// If the context does not have a deadline, the bigquery job will
// have no timeout.
func WaitForJob(ctx context.Context, job *bigquery.Job) (*bigquery.JobStatus, error) {
	waitCtx, cancel := shorten(ctx, time.Second*5)

	defer func() {
		// Cancel the waitCtx and release all resource.
		cancel()

		// Cancel the big query job if it has not finished.
		js, err := job.Status(ctx)
		if err != nil {
			// Non critical, just log.
			err = errors.Fmt("get bigquery status: %w", err)
			logging.Errorf(ctx, err.Error())
			return
		}
		if !js.Done() {
			err = job.Cancel(ctx)
			if err != nil {
				// Non critical, just log.
				err = errors.Fmt("cancel bigquery job: %w", err)
				logging.Errorf(ctx, err.Error())
			}
		}
	}()

	js, err := job.Wait(waitCtx)
	if err != nil {
		return nil, errors.Fmt("wait for job: %w", err)
	}
	return js, nil
}

// Shorten returns a derived context with its deadline shortened by d.
// The context can also be cancelled with the returned cancel function.
func shorten(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return context.WithCancel(ctx)
	}
	return context.WithDeadline(ctx, deadline.Add(-d))
}
