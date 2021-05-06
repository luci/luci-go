// Copyright 2021 The LUCI Authors.
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

// Package deadlineenforcer finalizes tasks with overdue deadlines.
package deadlineenforcer

import (
	"context"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/resultdb/internal/tasks"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/cron"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
)

const maxInvocationsPerShardToEnforceAtOnce = 100

// Options are for configuring the deadline enforcer.
type Options struct {
	// ForceCronInterval forces minimum interval in cron jobs.
	// Useful in integration tests to reduce the test time.
	ForceCronInterval time.Duration
}

// InitServer initializes a deadline enforcer server.
func InitServer(srv *server.Server, opts Options) {
	srv.RunInBackground("resultdb.purge", func(ctx context.Context) {
		minInterval := time.Minute
		if opts.ForceCronInterval > 0 {
			minInterval = opts.ForceCronInterval
		}
		cron.DynamicGroup(ctx, invocations.CurrentShardNumber, minInterval, enforceOneShard)
	})
}

func enforceOneShard(ctx context.Context, shard int) error {
	limit := maxInvocationsPerShardToEnforceAtOnce
	for {
		cnt, err := enforce(ctx, shard, limit)
		if err != nil {
			return err
		}
		if cnt == limit {
			// There's more invocations to finalize in this shard, keep going.
			continue
		}
		break
	}
	return nil
}

func enforce(ctx context.Context, shard, limit int) (int, error) {
	st := spanner.NewStatement(`
		SELECT InvocationId
		FROM Invocations@{FORCE_INDEX=InvocationsByActiveDeadline, spanner_emulator.disable_query_null_filtered_index_check=true}
		WHERE ShardId = @shardId
		AND ActiveDeadline <= CURRENT_TIMESTAMP()
		LIMIT @limit
	`)
	st.Params["shardId"] = shard
	st.Params["limit"] = limit
	var rowCount int32 = 0

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	err := spanutil.Query(ctx, st, func(row *spanner.Row) error {
		defer atomic.AddInt32(&rowCount, 1)
		var id invocations.ID
		if err := spanutil.FromSpanner(row, &id); err != nil {
			return err
		}
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			tasks.StartInvocationFinalization(ctx, id, true)
			return nil
		})
		return err
	})
	return int(rowCount), err
}
