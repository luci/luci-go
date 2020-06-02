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

package tasks

import (
	"context"
	"runtime"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/resultdb/internal/cron"
	"go.chromium.org/luci/resultdb/internal/invocations"
)

// Statuses of invocation tasks.
const (
	// The task completes successfully.
	success = "SUCCESS"

	// The task is skipped, possibly because another worker has leased it.
	skipped = "SKIPPED"

	// The task runs into a failure that can be resolved by retrying.
	transientFailure = "TRANSIENT_FAILURE"

	// The task runs into a permanent failure.
	permanentFailure = "PERMANENT_FAILURE"
)

var (
	attemptMetric = metric.NewCounter(
		"resultdb/task/attempts",
		"Counts of invocation task attempts.",
		nil,
		field.String("type"),   // see `Type`
		field.String("status")) // SUCCESS || TRANSIENT_FAILURE || PERMANENT_FAILURE

	durationMetric = metric.NewCumulativeDistribution(
		"resultdb/task/duration",
		"Distribution of an attempt’s execution duration.",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("type"),   // see `Type`
		field.String("status")) // SUCCESS || TRANSIENT_FAILURE || PERMANENT_FAILURE
)

// PermanentFailure set in an error indicates that the err is not resolvable by
// a retry. Such task is doomed.
var PermanentFailure = errors.BoolTag{
	Key: errors.NewTagKey("permanent failure to process invocation task"),
}

// TaskFunc can execute a task.
// If the returned error is tagged with PermanentFailure, then the failed task
// is deleted.
type TaskFunc func(ctx context.Context, invID invocations.ID, payload []byte) error

// Dispatcher queries for available tasks and dispatches them to goroutines.
type Dispatcher struct {
	// How often to query for tasks. Defaults to 1m.
	QueryInterval time.Duration

	// How long to lease a task for. Defaults to 1m.
	LeaseDuration time.Duration

	// Number of tasks to process concurrently. Defaults to GOMAXPROCS.
	Workers int
}

// runOne leases, runs and deletes one task.
// May return ErrConflict.
// Updates taskAttemptMetric and taskDurationMetric.
func (d *Dispatcher) runOne(ctx context.Context, taskType Type, id string, f TaskFunc) (err error) {
	// Update metrics.
	startTime := clock.Now(ctx)
	taskStatus := transientFailure
	defer func() {
		// Send metrics to tsmon.
		typeStr := string(taskType)

		duration := float64(time.Since(startTime).Milliseconds())
		durationMetric.Add(ctx, duration, typeStr, taskStatus)

		attemptMetric.Add(ctx, 1, typeStr, taskStatus)
	}()

	dur := d.LeaseDuration
	if dur <= 0 {
		dur = time.Minute
	}
	invID, payload, err := Lease(ctx, taskType, id, dur)
	if err != nil {
		if err == ErrConflict {
			taskStatus = skipped
		}
		return err
	}

	if err = f(ctx, invID, payload); err != nil {
		if PermanentFailure.In(err) {
			taskStatus = permanentFailure
			logging.Errorf(ctx, "permanent failure to process the task: %s", err)
			return Delete(ctx, taskType, id)
		}
		return err
	}

	// Invocation task is done, delete the row from spanner.
	if err := Delete(ctx, taskType, id); err != nil {
		return err
	}

	taskStatus = success
	return nil
}

// Run queries tasks and dispatches them to goroutines until ctx is canceled.
// Logs errors.
func (d *Dispatcher) Run(ctx context.Context, taskType Type, fn TaskFunc) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	queue := make(chan string)
	var wg sync.WaitGroup
	// Eventually stop the workers and wait for them to finish.
	defer func() {
		close(queue)
		wg.Wait()
	}()

	// Prepare workers to process the queue.
	workers := d.Workers
	if workers == 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for id := range queue {
				ctx := logging.SetField(ctx, "task_id", id)
				switch err := d.runOne(ctx, taskType, id, fn); {
				case err == ErrConflict:
					// It's possible another worker has leased the task, and it's fine, skip.
					logging.Warningf(ctx, "Conflict while trying to lease the task")

				case err != nil:
					// Do not bail on other task ids, otherwise we sample tasks too often
					// which is expensive. Just log the error and move on to the next task.
					if PermanentFailure.In(err) {
						logging.Errorf(ctx, "Task failed permanently: %s", err)
					} else {
						logging.Warningf(ctx, "Task failed transiently: %s", err)
					}

				default:
					logging.Infof(ctx, "Task succeeded")
				}
			}
		}()
	}

	// Stream available tasks in a loop and dispatch to workers.
	minInterval := d.QueryInterval
	if minInterval <= 0 {
		minInterval = time.Minute
	}
	cron.Run(ctx, minInterval, func(ctx context.Context) error {
		return Peek(ctx, taskType, func(id string) error {
			queue <- id
			return nil
		})
	})
}
