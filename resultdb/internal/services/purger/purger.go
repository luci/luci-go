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

// Package purger deletes expired test results from Spanner.
package purger

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/cron"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
)

// Options is purger server configuration.
type Options struct {
	// ForceCronInterval forces minimum interval in cron jobs.
	// Useful in integration tests to reduce the test time.
	ForceCronInterval time.Duration
}

// InitServer initializes a purger server.
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
		panic(errors.Fmt("failed to determine number of shards: %w", err))
	}

	// Start one cron job for each shard of the database.
	cron.Group(ctx, maxShard+1, minInterval, purgeOneShard)
}

func purgeOneShard(ctx context.Context, shard int) error {
	st := spanner.NewStatement(`
		SELECT InvocationId
		FROM Invocations@{FORCE_INDEX=InvocationsByExpectedTestResultsExpiration, spanner_emulator.disable_query_null_filtered_index_check=true}
		WHERE ShardId = @shardId
		AND ExpectedTestResultsExpirationTime IS NOT NULL
		AND ExpectedTestResultsExpirationTime <= CURRENT_TIMESTAMP()
	`)
	st.Params["shardId"] = shard
	return spanutil.Query(span.Single(ctx), st, func(row *spanner.Row) error {
		var id invocations.ID
		if err := spanutil.FromSpanner(row, &id); err != nil {
			return err
		}

		if err := purgeOneInvocation(ctx, id); err != nil {
			logging.Errorf(ctx, "failed to process %s: %s", id, err)
		}
		return nil
	})
}

func purgeOneInvocation(ctx context.Context, invID invocations.ID) error {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Check that invocation hasn't been purged already.
	var expirationTime spanner.NullTime
	var realm spanner.NullString
	err := invocations.ReadColumns(ctx, invID, map[string]any{
		"ExpectedTestResultsExpirationTime": &expirationTime,
		"Realm":                             &realm,
	})
	if err != nil {
		return err
	}
	if expirationTime.IsNull() {
		// Invocation was purged by other worker.
		return nil
	}

	// Stream rows that need to be purged and delete them in batches.
	// Note that we cannot use Partitioned UPDATE here because its time complexity
	// is currently O(table size).
	// Also Partitioned DML does not support JOINs which we need to purge both
	// test results and artifacts.
	var ms []*spanner.Mutation
	count := 0
	err = rowsToPurge(ctx, invID, func(table string, key spanner.Key) error {
		count++
		ms = append(ms, spanner.Delete(table, key))
		// Flush if the batch is too large.
		// Cloud Spanner limitation is 20k mutations per txn.
		// One deletion is one mutation.
		// Flush at 19k boundary.
		if len(ms) > 19000 {
			if _, err := span.Apply(ctx, ms); err != nil {
				return err
			}
			spanutil.IncRowCount(ctx, len(ms), spanutil.TestResults, spanutil.Deleted, realm.StringVal)
			ms = ms[:0]
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Flush the last batch.
	if len(ms) > 0 {
		if _, err := span.Apply(ctx, ms); err != nil {
			return err
		}
		spanutil.IncRowCount(ctx, len(ms), spanutil.TestResults, spanutil.Deleted, realm.StringVal)
	}

	// Set the invocation's result expiration to null.
	if err := unsetInvocationResultsExpiration(ctx, invID); err != nil {
		return err
	}

	logging.Debugf(ctx, "Deleted %d test results in %s", count, invID.Name())
	return nil
}

// rowsToPurge calls f for rows that should be purged.
func rowsToPurge(ctx context.Context, inv invocations.ID, f func(table string, key spanner.Key) error) error {
	st := spanner.NewStatement(`
		WITH DoNotPurge AS (
			SELECT DISTINCT TestId, VariantHash
			FROM TestResults@{FORCE_INDEX=UnexpectedTestResults, spanner_emulator.disable_query_null_filtered_index_check=true}
			WHERE InvocationId = @invocationId
			  AND IsUnexpected = TRUE
		)
		SELECT tr.TestId, tr.ResultId, art.ArtifactId
		FROM TestResults tr
		LEFT JOIN DoNotPurge dnp ON tr.TestId = dnp.TestId AND tr.VariantHash = dnp.VariantHash
		LEFT JOIN Artifacts art
		  ON art.InvocationId = tr.InvocationId AND FORMAT("tr/%s/%s", tr.TestId, tr.ResultId) = art.ParentId
		WHERE tr.InvocationId = @invocationId
			AND dnp.VariantHash IS NULL
	`)

	st.Params["invocationId"] = inv

	var lastTestID, lastResultID string
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var testID, resultID string
		var artifactID spanner.NullString
		if err := row.Columns(&testID, &resultID, &artifactID); err != nil {
			return err
		}

		// Given that we join by TestId and ResultId, result rows with the same
		// test id and result id will be contiguous.
		// This is not guaranteed, but happens in practice.
		// Even if we encounter (testID, resultID) that we've deleted before, this
		// is OK because a Spanner Delete ignores absence of the target row.
		// Ultimately, this is an optimization + code simplfication.
		if testID != lastTestID || resultID != lastResultID {
			if err := f("TestResults", inv.Key(testID, resultID)); err != nil {
				return err
			}

			lastTestID = testID
			lastResultID = resultID
		}

		if artifactID.Valid {
			parentID := artifacts.ParentID(testID, resultID)
			if err := f("Artifacts", inv.Key(parentID, artifactID)); err != nil {
				return err
			}
		}

		return nil
	})
}

func unsetInvocationResultsExpiration(ctx context.Context, id invocations.ID) error {
	_, err := span.Apply(ctx, []*spanner.Mutation{
		spanutil.UpdateMap("Invocations", map[string]any{
			"InvocationID":                      id,
			"ExpectedTestResultsExpirationTime": nil,
		}),
	})
	return err
}
