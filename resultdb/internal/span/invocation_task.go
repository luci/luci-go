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

type TaskRow struct {
	InvocationID   InvocationID
	TaskID         string
	InvocationTask *internalpb.InvocationTask
}

// SampleInvocationTasks randomly picks sampleSize of rows in InvocationTasks
// that are with ProcessAfter earlier than processTime.
func SampleInvocationTasks(ctx context.Context, processTime time.Time, sampleSize int64) ([]*TaskRow, error) {
	txn := Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	st := spanner.NewStatement(`
		SELECT
		 InvocationId,
		 TaskId,
     Payload,
		FROM
			(SELECT
				InvocationId,
				TaskId,
				Payload
			FROM
				InvocationTasks
			WHERE ProcessAfter <= @processTime)
		TABLESAMPLE RESERVOIR(@sampleSize ROWS)
	`)

	st.Params = ToSpannerMap(map[string]interface{}{
		"processTime": processTime,
		"sampleSize":  sampleSize,
	})

	ret := make([]*TaskRow, sampleSize)
	var b Buffer
	err := query(ctx, txn, st, func(row *spanner.Row) error {
		task := &TaskRow{}
		err := b.FromSpanner(row,
			task.InvocationID,
			task.TaskID,
			task.InvocationTask)

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
