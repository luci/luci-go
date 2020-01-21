// Copyright 2019 The LUCI Authors.
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

package span

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
)

// TaskKey represents the key to a specific invocation task.
type TaskKey struct {
	InvocationID InvocationID
	TaskID       string
}

// Key returns a Spanner key for the task.
func (tk *TaskKey) Key() spanner.Key {
	return tk.InvocationID.Key(tk.TaskID)
}

// InsertInvocationTask inserts one row to InvocationTasks.
func InsertInvocationTask(key TaskKey, invTask *internalpb.InvocationTask, processAfter time.Time) *spanner.Mutation {
	return InsertMap("InvocationTasks", map[string]interface{}{
		"InvocationId": key.InvocationID,
		"TaskID":       key.TaskID,
		"Payload":      invTask,
		"ProcessAfter": processAfter,
	})
}

// SampleInvocationTasks randomly picks sampleSize of rows in InvocationTasks
// with ProcessAfter earlier than processTime.
func SampleInvocationTasks(ctx context.Context, processTime time.Time, sampleSize int64) ([]TaskKey, error) {
	st := spanner.NewStatement(`
		WITH readyTasks AS
			(SELECT
				InvocationId,
				TaskId
			FROM InvocationTasks
			WHERE ProcessAfter <= @processTime)
		SELECT *
		FROM readyTasks
		TABLESAMPLE RESERVOIR(@sampleSize ROWS)
	`)

	st.Params = ToSpannerMap(map[string]interface{}{
		"processTime": processTime,
		"sampleSize":  sampleSize,
	})

	ret := make([]TaskKey, 0, sampleSize)
	var b Buffer
	err := Query(ctx, "sample inv tasks", Client(ctx).Single(), st, func(row *spanner.Row) error {
		task := TaskKey{}
		err := b.FromSpanner(row, &task.InvocationID, &task.TaskID)

		if err != nil {
			return err
		}
		ret = append(ret, task)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
