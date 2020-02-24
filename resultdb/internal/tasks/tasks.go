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

// Package tasks implements asynchronous invocation processing.
package tasks

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/span"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// Type is a value for InvocationTasks.TaskType column.
// It defines what a task does.
type Type string

// Key returns a Spanner key for the InvocationTasks row.
func (t Type) Key(taskID string) spanner.Key {
	return spanner.Key{string(t), taskID}
}

// Types of invocation tasks. Used as InvocationTasks.TaskType column value.
const (
	// BQExport is a type of task that exports an invocation to BigQuery.
	// The task payload is binary-encoded BigQueryExport message.
	BQExport Type = "bq_export"

	// TryFinalizeInvocation is a type of task that tries to finalize an
	// invocation. No payload.
	TryFinalizeInvocation Type = "finalize"
)

// AllTypes is a slice of all known types of tasks.
var AllTypes = []Type{BQExport, TryFinalizeInvocation}

// Enqueue inserts one row to InvocationTasks.
func Enqueue(typ Type, taskID string, invID span.InvocationID, payload interface{}, processAfter time.Time) *spanner.Mutation {
	internal.AssertUTC(processAfter)
	return span.InsertMap("InvocationTasks", map[string]interface{}{
		"TaskType":     string(typ),
		"TaskId":       taskID,
		"InvocationId": invID,
		"Payload":      payload,
		"CreateTime":   spanner.CommitTimestamp,
		"ProcessAfter": processAfter,
	})
}

// EnqueueBQExport inserts one row to InvocationTasks for a bq export task.
func EnqueueBQExport(invID span.InvocationID, payload *pb.BigQueryExport, processAfter time.Time) *spanner.Mutation {
	return Enqueue(BQExport, fmt.Sprintf("%s:0", invID.RowID()), invID, payload, processAfter)
}

// Peek calls f on available tasks of a given type.
func Peek(ctx context.Context, typ Type, f func(id string) error) error {
	st := spanner.NewStatement(`
		SELECT TaskId
		FROM InvocationTasks
		WHERE TaskType = @taskType AND ProcessAfter <= CURRENT_TIMESTAMP()
	`)

	st.Params["taskType"] = string(typ)

	var b span.Buffer
	return span.Query(ctx, span.Client(ctx).Single(), st, func(row *spanner.Row) error {
		var id string
		if err := b.FromSpanner(row, &id); err != nil {
			return err
		}
		return f(id)
	})
}

// ErrConflict is returned by Lease if the task does not exist or is already
// leased.
var ErrConflict = fmt.Errorf("the task is already leased")

// Lease leases an invocation task.
// If the task does not exist or is already leased, returns ErrConflict.
func Lease(ctx context.Context, typ Type, id string, duration time.Duration) (invID span.InvocationID, payload []byte, err error) {
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		now := clock.Now(ctx).UTC()
		var processAfter time.Time
		err := span.ReadRow(ctx, txn, "InvocationTasks", typ.Key(id), map[string]interface{}{
			"InvocationId": &invID,
			"ProcessAfter": &processAfter,
			"Payload":      &payload,
		})
		switch {
		case grpc.Code(err) == codes.NotFound:
			logging.Warningf(ctx, "task %s/%s not found: %s", typ, id, err)
			return ErrConflict

		case err != nil:
			return err

		case processAfter.After(now):
			return ErrConflict

		default:
			return txn.BufferWrite([]*spanner.Mutation{
				span.UpdateMap("InvocationTasks", map[string]interface{}{
					"TaskType":     string(typ),
					"TaskId":       id,
					"ProcessAfter": now.Add(duration),
				}),
			})
		}
	})
	return
}

// Delete deletes a task.
func Delete(ctx context.Context, typ Type, id string) error {
	_, err := span.Client(ctx).Apply(ctx, []*spanner.Mutation{
		spanner.Delete("InvocationTasks", typ.Key(id)),
	})
	return err
}
