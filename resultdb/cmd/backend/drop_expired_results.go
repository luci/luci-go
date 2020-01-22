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

// maxWorkers is the number of concurrent invocations to purge of expired
// results.
const maxWorkers = 30

// maxTestVariantsToFilter is the maximum number of test/variants to include
// in the exclusion clause of the paritioned delete statement used to purge
// expired test results. Invocations that have more than this number of test
// variant combinations with unexpected results will not be purged, until
// the whole invocation expires.
const maxTestVariantsToFilter = 1000

func unsetInvocationResultsExpiration(ctx context.Context, id *span.InvocationID) error {

	_, err := span.Client(ctx).Apply(ctx, []*spanner.Mutation{
		spanner.Update("Invocations", []string{"InvocationID", "ExpectedTestResultsExpirationTime"}, []interface{}{span.ToSpanner(*id), nil}),
	})
	return err
}

// dispatchExpiredResultDeletionTasks uses a pool of workers for purging results
// for several invocations in parallel
func dispatchExpiredResultDeletionTasks(ctx context.Context, expiredResultsInvocationIds []*span.InvocationID) error {
	return parallel.WorkPool(len(expiredResultsInvocationIds), func(workC chan<- func() error) {
		for _, id := range expiredResultsInvocationIds {
			id := id
			workC <- func() error {
				// Get the varianthashes for variants that have one or more unexpected results.
				unexpectedTestVariants, err := queryVariantsWithUnexpectedResults(ctx, id)
				if err != nil {
					return err
				}

				// Delete results for variants with only expected results.
				if err := deleteResultsFromExpectedVariants(ctx, id, unexpectedTestVariants); err != nil {
					return err
				}

				// Set the invocation's result expiration to null
				return unsetInvocationResultsExpiration(ctx, id)

			}
		}
	})
}

// dropExpiredResults is the continuous loop that repeatedly polls a random shard
// and purges expired test results for a number of its invocations.
func dropExpiredResults(ctx context.Context) {
	attempt := 0
	for {
		targetShard := mathrand.Int63n(ctx, span.InvocationShards)
		expiredResultsInvocationIds, err := sampleExpiredResultsInvocations(ctx, clock.Now(ctx), targetShard, maxWorkers)
		if err != nil {
			logging.Errorf(ctx, "Failed to query invocations with expired results: %s", err)

			attempt++
			sleep := time.Duration(attempt) * time.Second
			if sleep > 5*time.Second {
				sleep = 5 * time.Second
			}
			time.Sleep(sleep)

			continue
		}

		attempt = 0
		if len(expiredResultsInvocationIds) == 0 {
			// If the shard has no invocations with expired results, sleep for a bit.
			// This should make this job query often when there's many shards with
			// expired results in the database and throttle down when we have fewer.
			time.Sleep(30 * time.Second)
		}
		if err = dispatchExpiredResultDeletionTasks(ctx, expiredResultsInvocationIds); err != nil {
			logging.Errorf(ctx, "Failed to delete expired test results: %s", err)
		}
	}
}

// sampleExpiredResultsInvocations selects a random set of invocations in a
// given shard that have expired test results.
func sampleExpiredResultsInvocations(ctx context.Context, expirationTime time.Time, shardId, sampleSize int64) ([]*span.InvocationID, error) {
	st := spanner.NewStatement(`
	WITH expiringInvocations AS
		(SELECT InvocationId
		FROM Invocations
		WHERE ShardId = @shardId
		AND ExpectedTestResultsExpirationTime IS NOT NULL
		AND ExpectedTestResultsExpirationTime <= @expirationTime)
	SELECT *
	FROM expiringInvocations
	TABLESAMPLE RESERVOIR ( @sampleSize ROWS )
	`)
	st.Params = span.ToSpannerMap(map[string]interface{}{
		"expirationTime": expirationTime,
		"shardId":        shardId,
		"sampleSize":     sampleSize,
	})
	ret := make([]*span.InvocationID, 0, sampleSize)
	var b span.Buffer
	err := span.Query(ctx, "Sample expired results invocations", span.Client(ctx).Single(), st, func(row *spanner.Row) error {
		var id span.InvocationID
		err := b.FromSpanner(row, &id)

		if err != nil {
			return err
		}
		ret = append(ret, &id)
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
// test/variant combination given in testVariantsToKeep.
//
// This function will bail out, if too many test/variant combinations are given, as
// this could result in an exceedingly big delete statement.
func deleteResultsFromExpectedVariants(ctx context.Context, id *span.InvocationID, testVariantsToKeep []*testIdVariantHash) error {
	if len(testVariantsToKeep) > maxTestVariantsToFilter {
		logging.Infof(ctx, "Too many (%d) test/variants with unexpected test results for Invocation %s, not purging", len(testVariantsToKeep), id.Name())
		return nil
	} else {
		var st spanner.Statement
		if len(testVariantsToKeep) == 0 {
			st = spanner.NewStatement(`
				DELETE FROM TestResults
				WHERE InvocationId = @invocationId`)
			st.Params["invocationId"] = span.ToSpanner(*id)
		} else {
			st = spanner.NewStatement(`
				DELETE FROM TestResults
				WHERE InvocationId = @invocationId
				AND (TestId, VariantHash) NOT IN UNNEST(@testVariantsToKeep)`)
			st.Params["invocationId"] = span.ToSpanner(*id)
			st.Params["testVariantsToKeep"] = testVariantsToKeep
		}
		delRowsCount, err := span.Client(ctx).PartitionedUpdate(ctx, st)
		if err == nil {
			logging.Infof(ctx, "Deleted %d expired test result rows for Invocation %s", delRowsCount, id.Name())
		}
		return err
	}
}

// queryVariantsWithUnexpectedResults finds the test/variant combinations that
// have at least one unexpected test result for a given invocation.
func queryVariantsWithUnexpectedResults(ctx context.Context, id *span.InvocationID) ([]*testIdVariantHash, error) {
	testVariantsToKeep := []*testIdVariantHash{}
	st := spanner.NewStatement(`
		SELECT DISTINCT TestId, VariantHash
		FROM TestResults@{FORCE_INDEX=UnexpectedTestResults}
		WHERE InvocationId = @invocationId
		AND IsUnexpected = TRUE`)

	st.Params["invocationId"] = span.ToSpanner(*id)
	err := span.Query(ctx, "Query test/variant combinations with expired test results", span.Client(ctx).Single(), st, func(row *spanner.Row) error {
		currVariant := testIdVariantHash{}
		err := row.Columns(&currVariant.TestId, &currVariant.VariantHash)
		if err != nil {
			return err
		}
		testVariantsToKeep = append(testVariantsToKeep, &currVariant)
		return nil

	})
	if err != nil {
		return nil, err
	}
	return testVariantsToKeep, nil
}
