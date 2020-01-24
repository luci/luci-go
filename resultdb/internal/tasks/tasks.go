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
	return span.InsertMap("InvocationTasks", map[string]interface{}{
		"TaskType":     string(typ),
		"TaskId":       taskID,
		"InvocationId": invID,
		"Payload":      payload,
		"ProcessAfter": processAfter,
	})
}

// EnqueueBQExport inserts one row to InvocationTasks for a bq export task.
func EnqueueBQExport(invID span.InvocationID, payload *pb.BigQueryExport, processAfter time.Time) *spanner.Mutation {
	return Enqueue(BQExport, fmt.Sprintf("%s:0", invID), invID, payload, processAfter)
}

// Sample randomly picks sampleSize of tasks of a given type
// with ProcessAfter earlier than processTime.
func Sample(ctx context.Context, typ Type, processTime time.Time, sampleSize int64) ([]string, error) {
	st := spanner.NewStatement(`
		WITH readyTasks AS (
			SELECT TaskId
			FROM InvocationTasks
			WHERE TaskType = @taskType AND ProcessAfter <= @processTime
		)
		SELECT *
		FROM readyTasks
		TABLESAMPLE RESERVOIR(@sampleSize ROWS)
	`)

	st.Params = span.ToSpannerMap(map[string]interface{}{
		"taskType":    string(typ),
		"processTime": processTime,
		"sampleSize":  sampleSize,
	})

	ret := make([]string, 0, sampleSize)
	var b span.Buffer
	err := span.Query(ctx, "sample inv tasks", span.Client(ctx).Single(), st, func(row *spanner.Row) error {
		var id string
		if err := b.FromSpanner(row, &id); err != nil {
			return err
		}
		ret = append(ret, id)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// ErrConflict is returned by Lease if the task does not exist or is already
// leased.
var ErrConflict = fmt.Errorf("the task is already leased")

// Lease leases an invocation task if it can.
// If the task does not exist or is already leased, returns ErrConflict.
func Lease(ctx context.Context, typ Type, id string, duration time.Duration) (invID span.InvocationID, payload []byte, err error) {
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		now := clock.Now(ctx)
		var processAfter time.Time
		err := span.ReadRow(ctx, txn, "InvocationTasks", typ.Key(id), map[string]interface{}{
			"InvocationId": &invID,
			"ProcessAfter": &processAfter,
			"Payload":      &payload,
		})
		switch {
		case grpc.Code(err) == codes.NotFound:
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
