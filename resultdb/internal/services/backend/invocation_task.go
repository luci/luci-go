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
	"time"

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

	// The task runs into a failure that can be resolved by retrying.
	transientFailure = "TRANSIENT_FAILURE"

	// The task runs into a permanent failure.
	permanentFailure = "PERMANENT_FAILURE"
)

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

// runInvocationTasks gets invocation tasks and dispatches them to workers.
func (b *backend) runInvocationTasks(ctx context.Context, taskType tasks.Type) {
	workers := b.TaskWorkers
	if workers == 0 {
		workers = 1
	}
	b.cronGroup(ctx, workers, time.Minute, func(ctx context.Context, replica int) error {
		ids, err := tasks.Sample(ctx, taskType, time.Now(), 100)
		if err != nil {
			return errors.Annotate(err, "failed to query invocation tasks").Err()
		}

		for _, id := range ids {
			ctx := logging.SetField(ctx, "task_id", id)
			switch err := b.runTask(ctx, taskType, id); {

			case err == tasks.ErrConflict:
				// It's possible another worker has leased the task, and it's fine, skip.
				logging.Warningf(ctx, "Conflict while trying to lease the task")

			case err != nil:
				// Do not bail on other task ids, otherwise we sample tasks too often
				// which is expensive. Just log the error and move on to the next task.
				how := "transiently"
				if permanentInvocationTaskErrTag.In(err) {
					how = "permanently"
				}
				logging.Errorf(ctx, "Task failed %s: %s", how, err)

			default:
				logging.Infof(ctx, "Task succeeded")
			}
		}
		return nil
	})
}
