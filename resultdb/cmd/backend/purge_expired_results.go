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

package main

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/resultdb/internal/span"
)

// maxPurgeTestResultsWorkers is the number of concurrent invocations to purge of expired
// results.
const maxPurgeTestResultsWorkers = 30

// maxTestVariantsToFilter is the maximum number of test variants to include
// in the exclusion clause of the paritioned delete statement used to purge
// expired test results. Invocations that have more than this number of test
// variant combinations with unexpected results will not be purged, until
// the whole invocation expires.
const maxTestVariantsToFilter = 1000

func unsetInvocationResultsExpiration(ctx context.Context, id span.InvocationID) error {

	_, err := span.Client(ctx).Apply(ctx, []*spanner.Mutation{
		spanner.UpdateMap("Invocations", map[string]interface{}{
			"InvocationID":                      span.ToSpanner(id),
			"ExpectedTestResultsExpirationTime": nil,
		}),
	})
	return err
}

// purgeOneInvocation finds test variants with unexpected results and drops the
// complement, if there aren't too many of them.
func purgeOneInvocation(ctx context.Context, id span.InvocationID) error {

	// Get the test variants that have one or more unexpected results.
	testVariantsToKeep, err := queryVariantsWithUnexpectedResults(ctx, id)
	if err != nil {
		return err
	}

	if len(testVariantsToKeep) > maxTestVariantsToFilter {
		logging.Infof(ctx, "Too many (%d) test variants with unexpected test results for Invocation %s, not purging", len(testVariantsToKeep), id.Name())
		return nil
	} else {
		// Delete results for variants with only expected results.
		if err := deleteResultsFromExpectedVariants(ctx, id, testVariantsToKeep); err != nil {
			return err
		}
	}

	// Set the invocation's result expiration to null
	return unsetInvocationResultsExpiration(ctx, id)
}

// dispatchExpiredResultDeletionTasks uses a pool of workers for purging results
// for several invocations in parallel.
func dispatchExpiredResultDeletionTasks(ctx context.Context, expiredResultsInvocationIds []span.InvocationID) error {
	return parallel.FanOutIn(func(workC chan<- func() error) {
		for _, id := range expiredResultsInvocationIds {
			id := id
			workC <- func() error {
				return purgeOneInvocation(ctx, id)
			}
		}
	})
}

// purgeExpiredResults is the continuous loop that repeatedly polls a random shard
// and purges expired test results for a number of its invocations.
func purgeExpiredResults(ctx context.Context) {
	attempt := 0
	lastIterationStart := clock.Now(ctx).Add(-time.Hour)
	for {
		if lastIterationStart.Sub(clock.Now(ctx)) < time.Duration(5)*time.Second {
			// Last iteration started too recently, avoid spinning too fast.
			attempt++
			span.LinearBackoff(attempt, 5)
		}
		lastIterationStart = clock.Now(ctx)
		expiredResultsInvocationIds, err := sampleExpiredResultsInvocations(ctx, clock.Now(ctx), maxPurgeTestResultsWorkers)
		if err != nil {
			logging.Errorf(ctx, "Failed to query invocations with expired results: %s", err)
			continue
		}

		if len(expiredResultsInvocationIds) == 0 {
			// If the shard has no invocations with expired results, sleep for a bit.
			// This should make this job query often when there's many shards with
			// expired results in the database and throttle down when we have fewer.
			time.Sleep(30 * time.Second)
			continue
		}
		if err := dispatchExpiredResultDeletionTasks(ctx, expiredResultsInvocationIds); err != nil {
			logging.Errorf(ctx, "Failed to delete expired test results: %s", err)
			continue
		}
		// Only reset the attempt count when we have successfully purged some tasks.
		attempt = 0
	}
}

// sampleExpiredResultsInvocations selects a random set of invocations in a
// given shard that have expired test results.
func sampleExpiredResultsInvocations(ctx context.Context, expirationTime time.Time, sampleSize int64) ([]span.InvocationID, error) {
	var maxShard int64
	row, err := span.Client(ctx).Single().Query(ctx, spanner.NewStatement(`
		SELECT MAX(ShardId)
		FROM Invocations
	`)).Next()
	row.Columns(&maxShard)
	if err != nil {
		return nil, err
	}
	targetShard := mathrand.Int63n(ctx, maxShard+1)
	st := spanner.NewStatement(`
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
	st.Params = span.ToSpannerMap(map[string]interface{}{
		"shardId":    targetShard,
		"sampleSize": sampleSize,
	})
	ret := make([]span.InvocationID, 0, sampleSize)
	var b span.Buffer
	err = span.Query(ctx, "Sample expired results invocations", span.Client(ctx).Single(), st, func(row *spanner.Row) error {
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

// testIdVariantHash is a simple pair-of-strings struct with no spanner columnnames.
//
// It is used to pass these pairs as sql params in the UNNEST function in the
// statement composed by deleteResultsFromExpectedVariants below.
type testIdVariantHash struct {
	TestId      string `spanner:""`
	VariantHash string `spanner:""`
}

// deleteResultsFromExpectedVariants composes and executes a partitioned delete to drop
// all the test results in a given invocation except those matching any of the given
// test variant combination given in testVariantsToKeep.
func deleteResultsFromExpectedVariants(ctx context.Context, id span.InvocationID, testVariantsToKeep []*testIdVariantHash) error {
	st := spanner.NewStatement(`
		DELETE FROM TestResults
		WHERE InvocationId = @invocationId
		AND (TestId, VariantHash) NOT IN UNNEST(@testVariantsToKeep)
	`)
	st.Params["invocationId"] = span.ToSpanner(id)
	st.Params["testVariantsToKeep"] = testVariantsToKeep
	delRowsCount, err := span.Client(ctx).PartitionedUpdate(ctx, st)
	if err == nil {
		logging.Infof(ctx, "Deleted %d expired test result rows for %s", delRowsCount, id.Name())
	}
	return err
}

// queryVariantsWithUnexpectedResults finds the test variant combinations that
// have at least one unexpected test result for a given invocation.
func queryVariantsWithUnexpectedResults(ctx context.Context, id span.InvocationID) ([]*testIdVariantHash, error) {
	ret := []*testIdVariantHash{}
	st := spanner.NewStatement(`
		SELECT DISTINCT TestId, VariantHash
		FROM TestResults@{FORCE_INDEX=UnexpectedTestResults}
		WHERE InvocationId = @invocationId
		AND IsUnexpected = TRUE
		LIMIT @maxTestVariantsToFilter
	`)

	st.Params["invocationId"] = span.ToSpanner(id)
	st.Params["maxTestVariantsToFilter"] = maxTestVariantsToFilter + 1
	err := span.Query(ctx, "Query test variant combinations with expired test results", span.Client(ctx).Single(), st, func(row *spanner.Row) error {
		currVariant := testIdVariantHash{}
		if err := row.Columns(&currVariant.TestId, &currVariant.VariantHash); err != nil {
			return err
		}
		ret = append(ret, &currVariant)
		return nil

	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
