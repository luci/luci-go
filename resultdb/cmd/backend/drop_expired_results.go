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

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/resultdb/internal/span"
)

// maxWorkers is the number of concurrent invocations to purge of expired results.
const maxWorkers = 30

func unsetInvocationResultsExpiration(ctx context.Context, id *span.InvocationID) error {

	_, err := span.Client(ctx).Apply(ctx, []*spanner.Mutation{
		spanner.Update("Invocations", []string{"InvocationID", "ExpectedTestResultsExpirationTime"}, []interface{}{span.ToSpanner(id), nil}),
	})
	return err
}

func dispatchExpiredResultDeletionTasks(ctx context.Context, expiredResultsInvocationIds []*span.InvocationID) error {
	return parallel.WorkPool(len(expiredResultsInvocationIds), func(workC chan<- func() error) {
		for _, id := range expiredResultsInvocationIds {
			id := id
			workC <- func() error {
				// Get the varianthashes for variants that have one or more unexpected results.
				unexpectedVariantHashes, err := span.QueryVariantsWithUnexpectedResults(ctx, id)
				if err != nil {
					return err
				}

				// Delete results for variants with only expected results.
				if err := span.DeleteResultsFromExpectedVariants(ctx, id, unexpectedVariantHashes); err != nil {
					return err
				}

				// Set the invocation's result expiration to null
				return unsetInvocationResultsExpiration(ctx, id)
			}
		}
	})
}

func dropExpiredResults(ctx context.Context) {
	attempt := 0
	for {

		targetShard := mathrand.Int63n(ctx, span.InvocationShards)
		expiredResultsInvocationIds, err := span.SampleExpiredResultsInvocations(ctx, time.Now(), targetShard, maxWorkers)
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
			time.Sleep(30 * time.Second)
		}
		if err = dispatchExpiredResultDeletionTasks(ctx, expiredResultsInvocationIds); err != nil {
			logging.Errorf(ctx, "Failed to delete expired test results: %s", err)
		}
	}
}
