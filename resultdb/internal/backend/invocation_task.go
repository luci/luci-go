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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/resultdb/internal/tasks"
)

// permanentInvocationTaskErrTag set in an error indicates that the err is not
// resolvable by retry.
var permanentInvocationTaskErrTag = errors.BoolTag{
	Key: errors.NewTagKey("permanent failure to process invocation task"),
}

func dispatchInvocationTasks(ctx context.Context, taskType tasks.Type, ids []string) (processed []string, err error) {
	leaseDuration := time.Minute
	switch taskType {
	case tasks.TryFinalizeInvocation:
		leaseDuration = 10 * time.Second
	}

	taskStatus := transientFailure
	var mu sync.Mutex
	err = parallel.WorkPool(10, func(workC chan<- func() error) {
		for _, id := range ids {
			id := id
			ctx := logging.SetField(ctx, "task_id", id)
			workC <- func() (err error) {
				startTime := clock.Now(ctx)

				defer func() {
					// Send metrics to tsmon.
					typeStr := string(taskType)

					duration := float64(time.Since(startTime).Microseconds())
					durationMetric.Add(ctx, duration, typeStr, taskStatus)

					taskAttemptMetric.Add(ctx, 1, typeStr, taskStatus)

					// Annotate the returned error with the task id.
					if err != nil {
						err = errors.Annotate(err, "failed to process task %s", id).Err()
					}
				}()

				invID, payload, err := tasks.Lease(ctx, taskType, id, leaseDuration)
				switch {
				case err == tasks.ErrConflict:
					// It's possible another worker has leased the task, and it's fine, skip.
					return nil
				case err != nil:
					return err
				}

				switch taskType {
				case tasks.BQExport:
					err = exportResultsToBigQuery(ctx, invID, payload)
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

				mu.Lock()
				processed = append(processed, id)
				mu.Unlock()
				taskStatus = success
				return nil
			}
		}
	})
	return
}

// runInvocationTasks gets invocation tasks and dispatches them to workers.
func runInvocationTasks(ctx context.Context, taskType tasks.Type) {
	mCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go processingLoop(ctx, time.Minute, time.Minute, time.Minute, func(ctx context.Context) error {
		recordOldestTaskMetric(mCtx, taskType)
		return nil
	})

	processingLoop(ctx, time.Second, 5*time.Second, 10*time.Minute, func(ctx context.Context) error {
		ids, err := tasks.Sample(ctx, taskType, time.Now(), 100)
		if err != nil {
			return errors.Annotate(err, "failed to query invocation tasks").Err()
		}

		processed, err := dispatchInvocationTasks(ctx, taskType, ids)
		if len(processed) > 0 {
			logging.Infof(ctx, "processed tasks: %q", processed)
		}
		return err
	})
}
