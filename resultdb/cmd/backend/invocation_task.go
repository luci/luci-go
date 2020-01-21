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
	"fmt"
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

var errLeasingConflict = fmt.Errorf("the task is already leased")

// leaseInvocationTask leases an invocation task if it can.
// If the task could not be leased, returns errLeasingConflict.
func leaseInvocationTask(ctx context.Context, id string) (span.InvocationID, *internalpb.InvocationTask, error) {
	var invID span.InvocationID
	task := &internalpb.InvocationTask{}
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		now := clock.Now(ctx)
		var processAfter time.Time
		err := span.ReadRow(ctx, txn, "InvocationTasks", spanner.Key{id}, map[string]interface{}{
			"InvocationId": &invID,
			"ProcessAfter": &processAfter,
			"Payload":      task,
		})
		switch {
		case err != nil:
			return err

		case processAfter.After(now):
			return errLeasingConflict

		default:
			return txn.BufferWrite([]*spanner.Mutation{
				span.UpdateMap("InvocationTasks", map[string]interface{}{
					"TaskId":       id,
					"ProcessAfter": now.Add(taskLeaseTime),
				}),
			})
		}
	})
	return invID, task, err
}

func deleteInvocationTask(ctx context.Context, id string) error {
	_, err := span.Client(ctx).Apply(ctx, []*spanner.Mutation{
		spanner.Delete("InvocationTasks", spanner.Key{id}),
	})
	return err
}

func dispatchInvocationTasks(ctx context.Context, ids []string) error {
	return parallel.WorkPool(10, func(workC chan<- func() error) {
		for _, id := range ids {
			id := id
			workC <- func() error {
			invID, task, err := leaseInvocationTask(ctx, id)
				switch {
				case err == errLeasingConflict:
					// It's possible another worker has leased the task, and it's fine, skip.
					return nil
				case err != nil:
					return err
				}

				if err := exportResultsToBigQuery(ctx, invID, task.BigqueryExport); err != nil {
					if permanentInvocationTaskErrTag.In(err) {
						logging.Errorf(ctx, "permanent failure to process task %s: %s", id, err)
						return deleteInvocationTask(ctx, id)
					}
					return err
				}

				// Invocation task is done, delete the row from spanner.
				return deleteInvocationTask(ctx, id)
			}
		}
	})
}

// runInvocationTasks gets invocation tasks and dispatch the tasks to workers.
func runInvocationTasks(ctx context.Context) {
	// TODO(chanli): Add alert on failures.
	attempt := 0
	for {
		ids, err := span.SampleInvocationTasks(ctx, time.Now(), 100)
		if err != nil || len(ids) == 0 {
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
		if err = dispatchInvocationTasks(ctx, ids); err != nil {
			logging.Errorf(ctx, "Failed to run invocation tasks: %s", err)
		}
	}
}
