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
	"sync"
	"time"

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/resultdb/internal/tasks"
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

var taskSamplingLimit = rate.Every(8 * time.Second)

var (
	taskAttemptMetric = metric.NewCounter(
		"resultdb/task/attempts",
		"Counts of invocation task attempts.",
		nil,
		field.String("type"),   // tasks.Type
		field.String("status")) // SUCCESS || TRANSIENT_FAILURE || PERMANENT_FAILURE

	taskDurationMetric = metric.NewCumulativeDistribution(
		"resultdb/task/duration",
		"Distribution of an attempt’s execution duration.",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("type"),   // tasks.Type
		field.String("status")) // SUCCESS || TRANSIENT_FAILURE || PERMANENT_FAILURE
)

// permanentInvocationTaskErrTag set in an error indicates that the err is not
// resolvable by retry.
var permanentInvocationTaskErrTag = errors.BoolTag{
	Key: errors.NewTagKey("permanent failure to process invocation task"),
}

// runTask leases, runs and deletes a task.
// May return tasks.ErrConflict.
// Updates taskAttemptMetric and taskDurationMetric.
func (b *backend) runTask(ctx context.Context, taskType tasks.Type, id string) (err error) {
	leaseDuration := 10 * time.Minute
	switch taskType {
	case tasks.TryFinalizeInvocation:
		leaseDuration = time.Minute
	}
	if b.ForceLeaseDuration > 0 {
		leaseDuration = b.ForceLeaseDuration
	}

	// Update metrics.
	startTime := clock.Now(ctx)
	taskStatus := transientFailure
	defer func() {
		// Send metrics to tsmon.
		typeStr := string(taskType)

		duration := float64(time.Since(startTime).Milliseconds())
		taskDurationMetric.Add(ctx, duration, typeStr, taskStatus)

		taskAttemptMetric.Add(ctx, 1, typeStr, taskStatus)
	}()

	invID, payload, err := tasks.Lease(ctx, taskType, id, leaseDuration)
	if err != nil {
		if err == tasks.ErrConflict {
			taskStatus = skipped
		}
		return err
	}

	switch taskType {
	case tasks.BQExport:
		err = b.exportResultsToBigQuery(ctx, invID, payload)
	case tasks.TryFinalizeInvocation:
		err = tryFinalizeInvocation(ctx, invID)
	default:
		err = errors.Reason("unexpected task type %q", taskType).Err()
	}
	if err != nil {
		if permanentInvocationTaskErrTag.In(err) {
			taskStatus = permanentFailure
			logging.Errorf(ctx, "permanent failure to process the task: %s", err)
			return tasks.Delete(ctx, taskType, id)
		}
		return err
	}

	// Invocation task is done, delete the row from spanner.
	if err := tasks.Delete(ctx, taskType, id); err != nil {
		return err
	}

	taskStatus = success
	return nil
}

// runInvocationTasks queries invocation tasks and dispatches them to workers.
func (b *backend) runInvocationTasks(ctx context.Context, taskType tasks.Type) {
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
	workers := b.TaskWorkers
	if workers == 0 {
		workers = 1
	}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for id := range queue {
				ctx := logging.SetField(ctx, "task_id", id)
				switch err := b.runTask(ctx, taskType, id); {
				case err == tasks.ErrConflict:
					// It's possible another worker has leased the task, and it's fine, skip.
					logging.Warningf(ctx, "Conflict while trying to lease the task")

				case err != nil:
					// Do not bail on other task ids, otherwise we sample tasks too often
					// which is expensive. Just log the error and move on to the next task.
					if permanentInvocationTaskErrTag.In(err) {
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
	b.cron(ctx, 5*time.Second, func(ctx context.Context) error {
		return tasks.Peek(ctx, taskType, func(id string) error {
			queue <- id
			return nil
		})
	})
}
