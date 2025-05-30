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

	"go.chromium.org/luci/common/errors"
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

// FindRoots returns ALL root invocations for a given invocation.
// A root either
//   - has is_export_root = true, or
//   - has no parent invocation.
//
// If the given invocation is already a root, it will return a ID set that only contains the given invocation.
// If the given invocation has no root (eg. a <-> b), it will return an empty ID set.
//
// It will stop traverse the current path further once a root is encountered.
// For example, with graph a(is_export_root) -> b (is_export_root) -> c <- d,
// FindRoots(c) will return [b,d]. -> means includes.
func FindRoots(ctx context.Context, invID invocations.ID) (invocations.IDSet, error) {
	var isExportRoot spanner.NullBool
	err := invocations.ReadColumns(ctx, invID, map[string]any{"IsExportRoot": &isExportRoot})
	if err != nil {
		return nil, err
	}
	// Special case when the given invocation is an export root.
	if isExportRoot.Valid && isExportRoot.Bool {
		return invocations.NewIDSet(invID), nil
	}

	// Walk the graph of invocations, starting from the leaf, along the inclusion
	// edges to roots.
	// Store all visited invocations to avoid duplicates and cycles.
	visited := invocations.NewIDSet()
	visited.Add(invID)

	// findRoots recursively find all roots for an invocation.
	// TODO: This will run out of stack space if we have a very deep graph.
	// It is very unlikely, but if it happens we can rewrite it iteratively instead.
	var findRoots func(id invocations.ID) (invocations.IDSet, error)
	findRoots = func(id invocations.ID) (invocations.IDSet, error) {
		parents, err := queryParentInvocations(ctx, id)
		if err != nil {
			return nil, errors.Fmt("queryParentInvocations for %s: %w", id, err)
		}
		roots := invocations.NewIDSet()
		if len(parents) == 0 {
			// Invocation is a root of itself when it has no parent.
			roots.Add(id)
			return roots, nil
		}
		for _, parent := range parents {
			if _, ok := visited[parent.invocationID]; ok {
				// parent already visited, ignore.
				continue
			}
			visited.Add(parent.invocationID)
			if parent.isExportRoot {
				roots.Add(parent.invocationID)
				continue
			}
			// Roots of parent invocation are also roots of the current invocation.
			parentRoots, err := findRoots(parent.invocationID)
			if err != nil {
				return nil, err
			}
			roots.Union(parentRoots)
		}
		return roots, nil
	}
	return findRoots(invID)
}

type invocationInfo struct {
	invocationID invocations.ID
	isExportRoot bool
}

// queryParentInvocations returns all parents of a given invocation.
func queryParentInvocations(ctx context.Context, invID invocations.ID) ([]invocationInfo, error) {
	st := spanner.NewStatement(`
	SELECT
		iinv.InvocationId, inv.IsExportRoot
	FROM IncludedInvocations iinv
	JOIN Invocations inv ON iinv.InvocationId = inv.InvocationId
	WHERE IncludedInvocationId = @invocationID`)
	st.Params = spanutil.ToSpannerMap(spanutil.ToSpannerMap(map[string]any{
		"invocationID": invID,
	}))

	parents := make([]invocationInfo, 0)
	b := &spanutil.Buffer{}
	if err := span.Query(ctx, st).Do(func(r *spanner.Row) error {
		var invocationID invocations.ID
		var isExportRoot spanner.NullBool
		if err := b.FromSpanner(r, &invocationID, &isExportRoot); err != nil {
			return err
		}
		parents = append(parents, invocationInfo{
			invocationID: invocationID,
			isExportRoot: isExportRoot.Valid && isExportRoot.Bool,
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return parents, nil
}
