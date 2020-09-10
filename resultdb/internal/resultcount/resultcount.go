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

package resultcount

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
)

// Total number of shards for each invocation in TestResultCount table.
const totalShard = 10

func InitializeCounters(id invocations.ID) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, 0, totalShard)

	for i := 0; i < totalShard; i++ {
		ms = append(ms, spanutil.InsertMap("TestResultCounts", map[string]interface{}{
			"InvocationId":    id,
			"ShardId":         i,
			"TestResultCount": 0,
		}))
	}
	return ms
}

// IncrementTestResultCount increases the TestResultCounts of the invocation in one random shard.
func IncrementTestResultCount(ctx context.Context, id invocations.ID, delta int64) error {
	if delta == 0 {
		return nil
	}

	shardID := mathrand.Intn(ctx, totalShard)
	st := spanner.NewStatement(`
		UPDATE TestResultCounts
		SET TestResultCount = TestResultCount + @delta
		WHERE InvocationId = @invID
		AND ShardId = @shardID
	`)
	st.Params = spanutil.ToSpannerMap(map[string]interface{}{
		"invID":   id,
		"shardID": shardID,
		"delta":   delta,
	})
	switch rowCount, err := span.Update(ctx, st); {
	case err != nil:
		return err
	case rowCount != 1:
		return fmt.Errorf("resultcount: expected to update 1 row, updated %d row instead", rowCount)
	default:
		return nil
	}
}

// ReadTestResultCount returns the total number of test results of requested
// invocations.
func ReadTestResultCount(ctx context.Context, ids invocations.IDSet) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	st := spanner.NewStatement(`
		SELECT SUM(TestResultCount)
		FROM TestResultCounts
		WHERE InvocationId IN UNNEST(@invIDs)
	`)
	st.Params = spanutil.ToSpannerMap(map[string]interface{}{
		"invIDs": ids,
	})
	var count spanner.NullInt64
	err := spanutil.QueryFirstRow(ctx, st, &count)
	return count.Int64, err
}
