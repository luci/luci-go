// Copyright 2019 The LUCI Authors.
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

// Package invg contains types and functions to represent and fetch a graph of
// invocations.
package invg

import (
	"context"
	"sync"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/resultdb/internal/span"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// Graph is a directed graph of invocations.
// A node represents an invocation and is identified by a span.InvocationID.
// An edge (A, B) means invocation A includes invocation B.
//
// Not goroutine-safe.
type Graph struct {
	Nodes map[span.InvocationID]*Node
}

// Node stores invocation identity, data and outgoing edges.
type Node struct {
	ID         span.InvocationID
	Invocation *pb.Invocation
	Included   []span.InvocationID
}

// FetchGraph fetches invocations identified by roots and all invocations
// reachable from them.
//
// Current implementation is not optimized for long inclusion chains. In the
// worst case, RPCs are made sequentially.
// This can be optimized by caching a subset of the graph in Redis and populaing
// the rest from Spanner. This would increase parallelism and avoid fetching
// edges of a finalized invocations again.
// This optimization would require changing the function signature.
//
// If the returned error is non-nil, it is annotated with a gRPC code.
func FetchGraph(ctx context.Context, txn *spanner.ReadOnlyTransaction, roots ...span.InvocationID) (*Graph, error) {
	g := &Graph{
		Nodes: make(map[span.InvocationID]*Node, len(roots)),
	}
	var mu sync.Mutex

	var ensureNode func(id span.InvocationID)

	fetch := func(id span.InvocationID) (*Node, error) {
		inv, err := span.ReadInvocationFull(ctx, txn, id)
		if err != nil {
			return nil, err
		}

		ret := &Node{
			ID:         id,
			Invocation: inv,
			Included:   make([]span.InvocationID, len(inv.IncludedInvocations)),
		}
		for i, name := range inv.IncludedInvocations {
			id := span.MustParseInvocationName(name)
			ret.Included[i] = id
			ensureNode(id)
		}
		return ret, nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	ensureNode = func(id span.InvocationID) {
		mu.Lock()
		n := g.Nodes[id]
		if n != nil {
			mu.Unlock()
			return
		}

		n = &Node{ID: id}
		g.Nodes[id] = n
		mu.Unlock()

		// Concurrently fetch the node data without a lock.
		// Once we have it, lock and copy the data into n.
		eg.Go(func() error {
			data, err := fetch(id)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()

			n.Invocation = data.Invocation
			n.Included = data.Included
			return nil
		})
	}

	// Trigger fetching by requesting all roots.
	for _, id := range roots {
		ensureNode(id)
	}

	// Wait for the entire graph to be fetched.
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return g, nil
}
