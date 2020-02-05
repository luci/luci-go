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

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/tasks"
)

func queryOldestTaskAge(ctx context.Context, typ tasks.Type) (time.Time, error) {
	st := spanner.NewStatement(`
		SELECT CreateTime
		FROM InvocationTasks
		WHERE TaskType = @taskType
		ORDER BY CreateTime
		LIMIT 1
	`)

	st.Params = span.ToSpannerMap(map[string]interface{}{
		"taskType": string(typ),
	})

	var createTime time.Time
	err := span.QueryFirstRow(ctx, span.Client(ctx).Single(), st, &createTime)

	return createTime, err
}
