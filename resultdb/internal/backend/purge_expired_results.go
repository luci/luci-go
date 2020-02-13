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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/metric"

	"go.chromium.org/luci/resultdb/internal/span"
)

// maxTestVariantsToFilter is the maximum number of test variants to include
// in the exclusion clause of the paritioned delete statement used to purge
// expired test results. Invocations that have more than this number of test
// variant combinations with unexpected results will not be purged, until
// the whole invocation expires.
const maxTestVariantsToFilter = 1000

var purgedInvocationsMetric = metric.NewCounter(
	"resultdb/purged_invocations",
	"How many invocations have had their expected test results purged",
	nil)

func unsetInvocationResultsExpiration(ctx context.Context, id span.InvocationID) error {

	_, err := span.Client(ctx).Apply(ctx, []*spanner.Mutation{
		span.UpdateMap("Invocations", map[string]interface{}{
			"InvocationID":                      id,
			"ExpectedTestResultsExpirationTime": nil,
		}),
	})
	if err != nil {
		return err
	}
	purgedInvocationsMetric.Add(ctx, 1)
	return nil
}

// purgeOneInvocation finds test variants with unexpected results and drops the
// complement, if there aren't too many of them.
func purgeOneInvocation(ctx context.Context, id span.InvocationID) error {

	// Get the test variants that have one or more unexpected results (up to limit + 1).
	testVariantsToKeep, err := queryTestVariantsWithUnexpectedResults(ctx, id, maxTestVariantsToFilter+1)
	if err != nil {
		return err
	}

	if len(testVariantsToKeep) > maxTestVariantsToFilter {
		logging.Warningf(ctx, "Too many test variants with unexpected test results for %s, not purging", id.Name())
	} else if err := deleteTestResults(ctx, id, testVariantsToKeep); err != nil {
		return err
	}

	// Set the invocation's result expiration to null
	return unsetInvocationResultsExpiration(ctx, id)
}

// purgeExpiredResults purges expired test results and exports metrics.
// It blocks until context is canceled.
func (b *backend) purgeExpiredResults(ctx context.Context) {
	maxShard, err := span.CurrentMaxShard(ctx)
	switch {
	case err == span.ErrNoResults:
		maxShard = span.InvocationShards - 1
	case err != nil:
		panic(errors.Annotate(err, "failed to determine number of shards").Err())
	}

	// Start one cron job for each shard of the database.
	b.cronGroup(ctx, maxShard+1, time.Minute, func(ctx context.Context, shard int) error {
		id, err := randomExpiredResultsInvocation(ctx, shard)
		switch err {
		case span.ErrNoResults:
			return nil
		case nil:
			return purgeOneInvocation(ctx, id)
		default:
			return err
		}
	})
}

// randomExpiredResultsInvocation selects a random invocation that has expired
// test results.
func randomExpiredResultsInvocation(ctx context.Context, shardID int) (span.InvocationID, error) {
	st := spanner.NewStatement(`
		WITH expiringInvocations AS (
			SELECT InvocationId
			FROM Invocations@{FORCE_INDEX=InvocationsByExpectedTestResultsExpiration}
			WHERE ShardId = @shardId
			AND ExpectedTestResultsExpirationTime IS NOT NULL
			AND ExpectedTestResultsExpirationTime <= CURRENT_TIMESTAMP())
		SELECT *
		FROM expiringInvocations
		TABLESAMPLE RESERVOIR (1 ROWS)
	`)
	st.Params["shardId"] = shardID
	var ret span.InvocationID
	err := span.QueryFirstRow(ctx, span.Client(ctx).Single(), st, &ret)
	return ret, err
}

// testVariantID is a simple pair-of-strings struct with no spanner columnnames.
//
// It is used to pass these pairs as sql params in the UNNEST function in the
// statement composed by deleteTestResults below.
type testVariantID struct {
	TestId      string `spanner:""`
	VariantHash string `spanner:""`
}

// deleteTestResults composes and executes a partitioned delete to drop
// all the test results in a given invocation except those matching any of the given
// test variant combination given in except.
func deleteTestResults(ctx context.Context, id span.InvocationID, except []testVariantID) error {
	st := spanner.NewStatement(`
		DELETE FROM TestResults
		WHERE InvocationId = @invocationId
		AND (TestId, VariantHash) NOT IN UNNEST(@exceptTestVariants)
	`)
	st.Params["invocationId"] = span.ToSpanner(id)
	st.Params["exceptTestVariants"] = except
	count, err := span.Client(ctx).PartitionedUpdate(ctx, st)
	if err != nil {
		return err
	}
	logging.Infof(ctx, "Deleted %d expired test result rows in %s", count, id.Name())
	return nil
}

// queryTestVariantsWithUnexpectedResults finds up to `limit` test variant
// combinations that have at least one unexpected test result for a given
// invocation.
func queryTestVariantsWithUnexpectedResults(ctx context.Context, id span.InvocationID, limit int) ([]testVariantID, error) {
	ret := []testVariantID{}
	st := spanner.NewStatement(`
		SELECT DISTINCT TestId, VariantHash
		FROM TestResults@{FORCE_INDEX=UnexpectedTestResults}
		WHERE InvocationId = @invocationId
		AND IsUnexpected = TRUE
		LIMIT @limit
	`)

	st.Params["invocationId"] = id
	st.Params["limit"] = limit
	err := span.Query(ctx, span.Client(ctx).Single(), st, func(row *spanner.Row) error {
		tv := testVariantID{}
		if err := row.Columns(&tv.TestId, &tv.VariantHash); err != nil {
			return err
		}
		ret = append(ret, tv)
		return nil

	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
