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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
)

// Total number of shards for each invocation in TestResultCount table.
const nShards = 10

// IncrementTestResultCount increases the count in one random shard of the invocation.
func IncrementTestResultCount(ctx context.Context, id invocations.ID, delta int64) error {
	if delta == 0 {
		return nil
	}

	shardId := mathrand.Int63n(ctx, nShards)
	var count spanner.NullInt64
	err := spanutil.ReadRow(ctx, "TestResultCounts", id.Key(shardId), map[string]any{
		"TestResultCount": &count,
	})
	if err != nil && spanner.ErrCode(err) != codes.NotFound {
		return err
	}

	span.BufferWrite(ctx, spanutil.InsertOrUpdateMap("TestResultCounts", map[string]any{
		"InvocationId":    id,
		"ShardId":         shardId,
		"TestResultCount": count.Int64 + delta,
	}))
	return nil
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
	st.Params = spanutil.ToSpannerMap(map[string]any{
		"invIDs": ids,
	})
	var count spanner.NullInt64
	err := spanutil.QueryFirstRow(ctx, st, &count)
	return count.Int64, err
}
