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
	"fmt"
	"io"
	"sort"
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

// Node is one node in a directed graph of invocations.
type Node struct {
	ID            span.InvocationID
	Invocation    *pb.Invocation
	OutgoingEdges []*Edge
}

// Edge is one directed edge in a directed graph of invocations.
// The source node is known
type Edge struct {
	Included *Node
	Ready    bool
}

// FetchGraph fetches invocations identified by roots and all reachable
// invocations.
//
// Current implementation is vulnerable to long inclusion chains. In this case
// RPCs to resolve the full chain are done sequentially.
// This can be optimized by caching a subset of the graph in Redis and populate
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

	// schedules a goroutine to fetch outgoing edges for n, unless it was already
	// fetched.
	// Must be called with mu locked.
	var getNode func(id span.InvocationID) *Node

	fetch := func(id span.InvocationID) (*Node, error) {
		inv, err := span.ReadInvocationFull(ctx, txn, id)
		if err != nil {
			return nil, err
		}

		ret := &Node{
			ID:            id,
			Invocation:    inv,
			OutgoingEdges: make([]*Edge, 0, len(inv.Inclusions)),
		}
		for name, incl := range inv.Inclusions {
			includedInvID := span.MustParseInvocationName(name)
			included := getNode(includedInvID)
			ret.OutgoingEdges = append(ret.OutgoingEdges, &Edge{
				Included: included,
				Ready:    incl.Ready,
			})
		}
		sort.Slice(ret.OutgoingEdges, func(i, j int) bool {
			return ret.OutgoingEdges[i].Included.ID < ret.OutgoingEdges[j].Included.ID
		})
		return ret, nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	getNode = func(id span.InvocationID) *Node {
		mu.Lock()
		n := g.Nodes[id]
		if n != nil {
			mu.Unlock()
			return n
		}

		n = &Node{ID: id}
		g.Nodes[id] = n
		mu.Unlock()

		eg.Go(func() error {
			data, err := fetch(id)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()

			n.Invocation = data.Invocation
			n.OutgoingEdges = data.OutgoingEdges
			return nil
		})

		return n
	}

	for _, id := range roots {
		getNode(id)
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return g, nil
}

// Dump writes the contents of the graph in some not strictly defined, but
// deterministic format to w.
// Ignores Invocation proto fields.
func (g *Graph) Dump(w io.Writer) {
	ids := make([]span.InvocationID, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	for _, id := range ids {
		n := g.Nodes[id]
		fmt.Fprintf(w, "%s ->", n.ID)
		for _, incl := range n.OutgoingEdges {
			fmt.Fprint(w, " ")
			fmt.Fprint(w, incl.Included.ID)
		}
		fmt.Fprintln(w)
	}
}
