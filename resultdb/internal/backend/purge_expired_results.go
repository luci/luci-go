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

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/resultdb/internal/span"
)

// maxPurgeTestResultsWorkers is the number of concurrent invocations to purge of expired
// results.
const maxPurgeTestResultsWorkers = 100

// maxTestVariantsToFilter is the maximum number of test variants to include
// in the exclusion clause of the paritioned delete statement used to purge
// expired test results. Invocations that have more than this number of test
// variant combinations with unexpected results will not be purged, until
// the whole invocation expires.
const maxTestVariantsToFilter = 1000

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

// dispatchExpiredResultDeletionTasks uses a pool of workers for purging results
// for several invocations in parallel.
func dispatchExpiredResultDeletionTasks(ctx context.Context, invIDs []span.InvocationID) error {
	return parallel.FanOutIn(func(workC chan<- func() error) {
		for _, id := range invIDs {
			id := id
			workC <- func() error {
				return purgeOneInvocation(ctx, id)
			}
		}
	})
}

// purgeExpiredResults is a loop that repeatedly polls a random shard and purges
// expired test results for a number of its invocations.
func purgeExpiredResults(ctx context.Context) {
	recordExpiredResultsDelayMetric(ctx)
	processingLoop(ctx, 5*time.Second, 10*time.Minute, func(ctx context.Context) error {
		expiredResultsInvocationIds, err := sampleExpiredResultsInvocations(ctx, maxPurgeTestResultsWorkers)
		if err != nil {
			return err
		}
		if len(expiredResultsInvocationIds) == 0 {
			return nil
		}
		return dispatchExpiredResultDeletionTasks(ctx, expiredResultsInvocationIds)
	})
}

// sampleExpiredResultsInvocations selects a random set of invocations that have
// expired test results.
func sampleExpiredResultsInvocations(ctx context.Context, sampleSize int64) ([]span.InvocationID, error) {
	st := spanner.NewStatement(`
		SELECT ShardId
		FROM Invocations@{FORCE_INDEX=InvocationsByInvocationExpiration}
		ORDER BY ShardID DESC
		LIMIT 1
	`)
	var maxShard int64
	if err := span.QueryFirstRow(ctx, span.Client(ctx).Single(), st, &maxShard); err != nil {
		return nil, err
	}
	st = spanner.NewStatement(`
		WITH expiringInvocations AS (
			SELECT InvocationId
			FROM Invocations@{FORCE_INDEX=InvocationsByExpectedTestResultsExpiration}
			WHERE ShardId = @shardId
			AND ExpectedTestResultsExpirationTime IS NOT NULL
			AND ExpectedTestResultsExpirationTime <= CURRENT_TIMESTAMP())
		SELECT *
		FROM expiringInvocations
		TABLESAMPLE RESERVOIR (@sampleSize ROWS)
	`)
	st.Params["shardId"] = mathrand.Int63n(ctx, maxShard+1)
	st.Params["sampleSize"] = sampleSize
	ret := make([]span.InvocationID, 0, sampleSize)
	var b span.Buffer
	err := span.Query(ctx, span.Client(ctx).Single(), st, func(row *spanner.Row) error {
		var id span.InvocationID
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
