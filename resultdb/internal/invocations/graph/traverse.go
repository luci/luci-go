// Copyright 2023 The LUCI Authors.
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

// Package graph contains methods to explore reachable invocations.
package graph

import (
	"context"
	"sync"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
)

// FindInheritSourcesDescendants finds and return all invocations (including the root)
// which inherit commit source information from the root.
func FindInheritSourcesDescendants(ctx context.Context, invID invocations.ID) (invocations.IDSet, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Walk the graph of invocations, starting from the root, along the inclusion
	// edges.
	seen := make(invocations.IDSet, 1)
	var mu sync.Mutex

	// Limit the number of concurrent queries.
	sem := semaphore.NewWeighted(64)

	eg, ctx := errgroup.WithContext(ctx)

	var visit func(id invocations.ID)
	visit = func(id invocations.ID) {
		// Do not visit same node twice.
		mu.Lock()
		if seen.Has(id) {
			mu.Unlock()
			return
		}
		seen.Add(id)
		mu.Unlock()

		// Concurrently fetch inclusions without a lock.
		eg.Go(func() error {
			// Limit concurrent Spanner queries.
			if err := sem.Acquire(ctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)

			st := spanner.NewStatement(`
				SELECT included.InvocationId
				FROM IncludedInvocations incl
				JOIN Invocations included on incl.IncludedInvocationId = included.InvocationId
				WHERE incl.InvocationId = @invID AND included.InheritSources
			`)
			st.Params = spanutil.ToSpannerMap(map[string]any{"invID": id})
			var b spanutil.Buffer
			return span.Query(ctx, st).Do(func(row *spanner.Row) error {
				var includedID invocations.ID
				if err := b.FromSpanner(row, &includedID); err != nil {
					return err
				}
				visit(includedID)
				return nil
			})
		})
	}

	visit(invID)

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return seen, nil
}
