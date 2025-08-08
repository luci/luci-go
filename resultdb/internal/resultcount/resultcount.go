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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
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

// BatchIncrementTestResultCount increases the counts of test results in a random shard
// of the given invocations.
func BatchIncrementTestResultCount(ctx context.Context, deltas map[invocations.ID]int64) (err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.resultcount.BatchIncrementTestResultCount")
	defer func() { tracing.End(ts, err) }()

	idsToUpdate := invocations.NewIDSet()
	for id, delta := range deltas {
		if delta != 0 {
			idsToUpdate.Add(id)
		}
	}
	if len(idsToUpdate) == 0 {
		return nil
	}

	// Pick one shard and write to it for all invocations. This reduces the
	// collision risk compared to picking different shards for each invocation,
	// especially if there are other tasks also updating batches of invocations.
	shardId := mathrand.Int63n(ctx, nShards)
	var ms []*spanner.Mutation

	// Try to read existing state of count rows and update them if they exist.
	var b spanutil.Buffer
	err = span.Read(ctx, "TestResultCounts", idsToUpdate.Keys(shardId), []string{"InvocationId", "TestResultCount"}).Do(func(r *spanner.Row) error {
		var id invocations.ID
		var count int64
		if err := b.FromSpanner(r, &id, &count); err != nil {
			return errors.Fmt("parse row: %w", err)
		}
		ms = append(ms, spanutil.UpdateMap("TestResultCounts", map[string]any{
			"InvocationId":    id,
			"ShardId":         shardId,
			"TestResultCount": count + deltas[id],
		}))
		idsToUpdate.Remove(id)
		return nil
	})
	if err != nil {
		return errors.Fmt("read existing counts: %w", err)
	}

	// For any invocations not found in the initial read, insert them.
	for id := range idsToUpdate {
		delta := deltas[id]
		ms = append(ms, spanutil.InsertMap("TestResultCounts", map[string]any{
			"InvocationId":    id,
			"ShardId":         shardId,
			"TestResultCount": delta,
		}))
	}
	span.BufferWrite(ctx, ms...)
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
