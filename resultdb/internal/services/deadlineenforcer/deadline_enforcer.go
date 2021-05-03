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
	"time"

	"go.chromium.org/luci/resultdb/internal/tasks"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/cron"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
)

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
		run(ctx, minInterval)
	})
}

// run continuously purges expired test results.
// It blocks until context is canceled.
func run(ctx context.Context, minInterval time.Duration) {
	maxShard, err := invocations.CurrentMaxShard(ctx)
	switch {
	case err == spanutil.ErrNoResults:
		maxShard = invocations.Shards - 1
	case err != nil:
		panic(errors.Annotate(err, "failed to determine number of shards").Err())
	}

	// Start one cron job for each shard of the database.
	cron.Group(ctx, maxShard+1, minInterval, enforceOneShard)
}

func enforceOneShard(ctx context.Context, shard int) error {
	st := spanner.NewStatement(`
		SELECT InvocationId
		FROM Invocations@{FORCE_INDEX=InvocationsByActiveDeadline, spanner_emulator.disable_query_null_filtered_index_check=true}
		WHERE ShardId = @shardId
		AND ActiveDeadline <= CURRENT_TIMESTAMP()
		LIMIT 100
	`)
	st.Params["shardId"] = shard
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		return spanutil.Query(ctx, st, func(row *spanner.Row) error {
			var id invocations.ID
			if err := spanutil.FromSpanner(row, &id); err != nil {
				return err
			}
			tasks.StartInvocationFinalization(ctx, id, true)
			return nil
		})
	})
	return err
}
