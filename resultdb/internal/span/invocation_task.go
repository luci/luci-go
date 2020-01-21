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

// InsertInvocationTask inserts one row to InvocationTasks.
func InsertInvocationTask(taskID string, invID InvocationID, invTask *internalpb.InvocationTask, processAfter time.Time) *spanner.Mutation {
	return InsertMap("InvocationTasks", map[string]interface{}{
		"TaskId":       taskID,
		"InvocationId": invID,
		"Payload":      invTask,
		"ProcessAfter": processAfter,
	})
}

// SampleInvocationTasks randomly picks sampleSize of rows in InvocationTasks
// with ProcessAfter earlier than processTime.
func SampleInvocationTasks(ctx context.Context, processTime time.Time, sampleSize int64) ([]string, error) {
	st := spanner.NewStatement(`
		WITH readyTasks AS (
			SELECT TaskId
			FROM InvocationTasks
			WHERE ProcessAfter <= @processTime
		)
		SELECT *
		FROM readyTasks
		TABLESAMPLE RESERVOIR(@sampleSize ROWS)
	`)

	st.Params = ToSpannerMap(map[string]interface{}{
		"processTime": processTime,
		"sampleSize":  sampleSize,
	})

	ret := make([]string, 0, sampleSize)
	var b Buffer
	err := Query(ctx, "sample inv tasks", Client(ctx).Single(), st, func(row *spanner.Row) error {
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
