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

	"go.chromium.org/luci/common/sync/parallel"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
)

// TODO(crbug.com/1013316): remove when Realms are implemented.
const luciProject = "chromium"

// Buffer time for leasing an invocation task.
const taskLeasingTime = 10 * time.Minute

func readInvocationTask(ctx context.Context, txn span.Txn, taskKey *span.TaskKey) (time.Time, *internalpb.InvocationTask, error) {
	var processAfter time.Time
	task := &internalpb.InvocationTask{}
	err := span.ReadRow(ctx, txn, "InvocationTasks", taskKey.InvocationID.Key(taskKey.TaskID), map[string]interface{}{
		"ProcessAfter": &processAfter,
		"Payload":      task,
	})
	return processAfter, task, err
}

func updateInvocationTask(txn *spanner.ReadWriteTransaction, newProcessAfter time.Time, taskKey *span.TaskKey) error {
	return txn.BufferWrite([]*spanner.Mutation{
		span.UpdateMap("InvocationTasks", map[string]interface{}{
			"InvocationId": taskKey.InvocationID,
			"TaskId":       taskKey.TaskID,
			"ProcessAfter": newProcessAfter,
		}),
	})
}

// leaseInvocationTask leases an invocation task if it can.
func leaseInvocationTask(ctx context.Context, now time.Time, taskKey *span.TaskKey) (*internalpb.InvocationTask, bool, error) {
	shouldRunTask := true
	var task *internalpb.InvocationTask
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		var processAfter time.Time
		var err error
		processAfter, task, err = readInvocationTask(ctx, txn, taskKey)
		if err == nil && !processAfter.After(now) {
			return updateInvocationTask(txn, now.Add(taskLeasingTime), taskKey)
		}
		shouldRunTask = false
		return err
	})
	return task, shouldRunTask, err
}

func deleteInvocationTask(ctx context.Context, taskKey *span.TaskKey) error {
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		return txn.BufferWrite([]*spanner.Mutation{
			spanner.Delete("InvocationTasks", taskKey.InvocationID.Key(taskKey.TaskID)),
		})
	})
	return err
}

func dispatchInvocationTasks(ctx context.Context, taskKeys []*span.TaskKey) error {
	now := time.Now()

	return parallel.WorkPool(10, func(workC chan<- func() error) {
		for _, taskKey := range taskKeys {
			workC <- func() error {
				task, shouldRunTask, err := leaseInvocationTask(ctx, now, taskKey)

				if err != nil {
					return err
				}
				if !shouldRunTask {
					// It's possible another worker has leased the task, and it's fine, skip.
					return nil
				}

				if err = exportResultsToBigQuery(ctx, luciProject, taskKey.InvocationID, task.BigqueryExport); err != nil {
					return err
				}

				// Invocation task is done, delete the row from spanner.
				return deleteInvocationTask(ctx, taskKey)
			}
		}
	})
}
