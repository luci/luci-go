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

package main

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
)

// taskLeaseTime is the time allowed for a worker to work on an invocation task.
const taskLeaseTime = 10 * time.Minute

// permanentInvocationTaskErrTag set in an error indicates that the err is not
// resolvable by retry.
var permanentInvocationTaskErrTag = errors.BoolTag{
	Key: errors.NewTagKey("permanent failure to process invocation task"),
}

// leaseInvocationTask leases an invocation task if it can.
func leaseInvocationTask(ctx context.Context, taskKey span.TaskKey) (*internalpb.InvocationTask, bool, error) {
	shouldRunTask := true
	task := &internalpb.InvocationTask{}
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		now := clock.Now(ctx)
		var processAfter time.Time
		err := span.ReadRow(ctx, txn, "InvocationTasks", taskKey.Key(), map[string]interface{}{
			"ProcessAfter": &processAfter,
			"Payload":      task,
		})
		if err != nil || processAfter.After(now) {
			shouldRunTask = false
			return err
		}
		return txn.BufferWrite([]*spanner.Mutation{
			span.UpdateMap("InvocationTasks", map[string]interface{}{
				"InvocationId": taskKey.InvocationID,
				"TaskId":       taskKey.TaskID,
				"ProcessAfter": now.Add(taskLeaseTime),
			}),
		})
	})
	return task, shouldRunTask, err
}

func deleteInvocationTask(ctx context.Context, taskKey span.TaskKey) error {
	_, err := span.Client(ctx).Apply(ctx, []*spanner.Mutation{
		spanner.Delete("InvocationTasks", taskKey.Key()),
	})
	return err
}

func dispatchInvocationTasks(ctx context.Context, taskKeys []span.TaskKey) error {
	return parallel.WorkPool(10, func(workC chan<- func() error) {
		for _, taskKey := range taskKeys {
			taskKey := taskKey
			workC <- func() error {
				task, shouldRunTask, err := leaseInvocationTask(ctx, taskKey)

				if err != nil {
					return err
				}
				if !shouldRunTask {
					// It's possible another worker has leased the task, and it's fine, skip.
					return nil
				}

				if err := exportResultsToBigQuery(ctx, taskKey.InvocationID, task.BigqueryExport); err != nil {
					if permanentInvocationTaskErrTag.In(err) {
						logging.Errorf(ctx, "permanent failure to process task %s/%s: %s", taskKey.InvocationID, taskKey.TaskID, err)
						return deleteInvocationTask(ctx, taskKey)
					}
					return err
				}

				// Invocation task is done, delete the row from spanner.
				return deleteInvocationTask(ctx, taskKey)
			}
		}
	})
}

// runInvocationTasks gets invocation tasks and dispatch the tasks to workers.
func runInvocationTasks(ctx context.Context) {
	// TODO(chanli): Add alert on failures.
	attempt := 0
	for {
		taskKeys, err := span.SampleInvocationTasks(ctx, time.Now(), 100)
		if err != nil || len(taskKeys) == 0 {
			if err != nil {
				logging.Errorf(ctx, "Failed to query invocation tasks: %s", err)
			}

			attempt++
			sleep := time.Duration(attempt) * time.Second
			if sleep > 5*time.Second {
				sleep = 5 * time.Second
			}
			time.Sleep(sleep)

			continue
		}

		attempt = 0
		if err = dispatchInvocationTasks(ctx, taskKeys); err != nil {
			logging.Errorf(ctx, "Failed to run invocation tasks: %s", err)
		}
	}
}
