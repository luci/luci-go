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

package submission

import (
	"fmt"
	"sort"

	"go.chromium.org/luci/common/data/stringset"
)

// OptimizeOrders computes best order for submission of CLs based on their
// dependencies.
// TODO(yiwzhang): implements this once the model for cl finalized.
// We could either remove the node/edge structs and use cl struct
// directly or wrap the cl struct inside node.
func OptimizeOrders(cls interface{}) interface{} {
	panic("not implemented")
}

type node struct {
	key      string
	outEdges []edge
}

type edge struct {
	destKey string
	hard    bool
}

// optimize returns nodes in the order approximately minimizing the number of
// broken soft edges for the given directed graph.
//
// Panics if nodes contain duplicate keys or the hard edge solely can form a
// cycle. In case of a cycle introduced by soft edges, breaks cycle by
// arbitrarily but deterministically ignoring soft edge(s).
//
// Overall, this is as hard as a minimum feedback arc set, which is NP-Hard:
// https://en.wikipedia.org/wiki/Feedback_arc_set#Minimum_feedback_arc_set
// However, approximation is fine within CQ context so long as hard edges
// are obeyed.
func optimize(nodes []node) []node {
	return (&optimizer{}).optimize(nodes)
}

type optimizer struct {
	unvisitedNodes map[string]node
	removedEdges   map[edgeKey]struct{}
	cycleDetector  stringset.Set
	result         []node
}

type edgeKey struct {
	src, dest string
}

// This is dfs-based toposort which backtracks if cycle is found and deletes
// the latest followed edge with `Hard` == false.

// Denoting V=len(nodes), E=number of dependencies edges (bounded by O(V^2)),
// the runtime is bounded by O(V*E), which worst case can be O(V**3).
// In CQ context, E is usually O(V), so this is fine.
//
// This can be reduced to O(max(E, V)) by first computing isomorphic
// graph of connected components via `Hard` == true edges and then
// running toposorts on a graph of components AND inside each component
// separately.
// Alternative idea for improvement is to could accumulate a skiplist style
// datastructure during iteration too, i.e. from node X what is the next
// guaranteed-reachable node Y. Then during backtracking, just skip forward
// over chains of hard_requirement changes. Also, with a reciprocal structure,
// during 'skip back' phase one can skip directly to the nearest soft edge.
func (t *optimizer) optimize(nodes []node) []node {
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].key < nodes[j].key
	})
	t.unvisitedNodes = map[string]node{}
	for _, n := range nodes {
		if _, ok := t.unvisitedNodes[n.key]; ok {
			panic(fmt.Sprintf("duplicate key: %s", n.key))
		}
		t.unvisitedNodes[n.key] = n
	}

	t.removedEdges = map[edgeKey]struct{}{}
	t.cycleDetector = stringset.Set{}
	t.result = make([]node, 0, len(nodes))

	for _, n := range nodes {
		if t.visit(n) {
			// A cycle was formed via hard edges.
			panic(fmt.Sprintf("cycle detected for key: %s", n.key))
		}
	}
	return t.result
}

func (t *optimizer) visit(n node) bool {
	if _, ok := t.unvisitedNodes[n.key]; !ok {
		return false
	}
	if t.cycleDetector.Has(n.key) {
		return true
	}
	t.cycleDetector.Add(n.key)

	for _, e := range t.remainingEdges(n) {
		for t.visit(t.unvisitedNodes[e.destKey]) {
			// Going deeper forms a cycle which can't be broken up at deeper levels.
			if e.hard {
				// Can't break it at this level either, backtrack.
				t.cycleDetector.Del(n.key)
				return true
			}
			// Greedily break cycle by removing this edge. This is why algo is
			// only an approximation.
			t.removedEdges[edgeKey{n.key, e.destKey}] = struct{}{}
			break
		}
	}
	t.cycleDetector.Del(n.key)
	delete(t.unvisitedNodes, n.key)
	t.result = append(t.result, n)
	return false
}

func (t *optimizer) remainingEdges(n node) []edge {
	ret := make([]edge, 0, len(n.outEdges))
	for _, edge := range n.outEdges {
		if _, ok := t.unvisitedNodes[edge.destKey]; ok {
			if _, ok := t.removedEdges[edgeKey{n.key, edge.destKey}]; !ok {
				ret = append(ret, edge)
			}
		}
	}
	return ret
}
