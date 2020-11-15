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

package filegraph

import "container/heap"

// Query finds shortest paths from any of Query.Sources to other nodes,
// producing a shortest-path tree
// (https://en.wikipedia.org/wiki/Shortest-path_tree).
//
// It is based on Dijkstra's algorithm
// (https://en.wikipedia.org/wiki/Dijkstra's_algorithm).
type Query struct {
	// Sources are the nodes to start from.
	Sources []Node

	// MaxDistance, if positive, is the distance threshold.
	// Nodes further than this are considered unreachable.
	MaxDistance float64

	// TODO(crbug.com/1136280): add Backwards
	// TODO(crbug.com/1136280): add SilbingDistance

	heap spHeap
	dist map[Node]float64
}

// ShortestPath represents the shortest path from one of sources to a node.
// ShortestPath itself is a node in a shortest path tree
// (https://en.wikipedia.org/wiki/Shortest-path_tree).
type ShortestPath struct {
	Node     Node
	Distance float64
	// Prev points to the previous node on the path from a source to this node.
	// It can be used to reconstruct the whole shortest path, starting from a
	// source.
	Prev *ShortestPath
}

// Run calls the callback for each node reachable from any of the sources.
// The nodes are reported in the distance-ascending order relative to the
// sources, starting from the sources themselves.
//
// If the callback returns false, then the iteration stops.
func (q *Query) Run(callback func(*ShortestPath) (keepGoing bool)) {
	// This function implements Dijkstra's algorithm.

	q.heap = q.heap[:0]

	// Maps from a node to the shortest distance. Distance may shrink over time.
	if q.dist == nil {
		q.dist = map[Node]float64{}
	} else {
		// Note: the compiler optimizes this loop
		// https://go-review.googlesource.com/c/go/+/110055
		for k := range q.dist {
			delete(q.dist, k)
		}
	}

	// Add all sources to q.heap and dist.
	for _, n := range q.Sources {
		if n == nil {
			panic("one of the sources is nil")
		}
		if _, ok := q.dist[n]; !ok {
			q.heap = append(q.heap, &ShortestPath{Node: n})
			q.dist[n] = 0
		}
	}

	for len(q.heap) > 0 {
		cur := heap.Pop(&q.heap).(*ShortestPath)

		// Check the distance.
		switch {
		case q.MaxDistance > 0 && cur.Distance > q.MaxDistance:
			// This and all subsequent nodes are too far.
			return

		case cur.Distance > q.dist[cur.Node]:
			// A better one was already reported.
			continue
		}

		if !callback(cur) {
			return
		}

		consider := func(other Node, distFromCur float64) bool {
			newDist := cur.Distance + distFromCur
			if curDist, ok := q.dist[other]; !ok || newDist < curDist {
				q.dist[other] = newDist
				// If heap already contains the node, we cannot efficiently reduce
				// its distance. Instead we push a new entry, and the previous (worse)
				// one will be filtered out later.
				heap.Push(&q.heap, &ShortestPath{
					Prev:     cur,
					Node:     other,
					Distance: newDist,
				})
			}
			return true
		}

		cur.Node.Outgoing(consider)

		// TODO(crbug.com/1136280): use q.SiblingDistance and call consider()
	}
}

// ShortestPath returns the shortest path to a node.
// Returns nil if the path does not exist.
// ShortestPath.Path() can be used to reconstruct the full path.
func (q *Query) ShortestPath(to Node) *ShortestPath {
	if to == nil {
		panic("to is nil")
	}

	var ret *ShortestPath
	q.Run(func(result *ShortestPath) (keepGoing bool) {
		if result.Node != to {
			return true
		}

		ret = result
		return false
	})
	return ret
}

// Path reconstructs the path from a query source to r.Node.
func (r *ShortestPath) Path() []*ShortestPath {
	var ret []*ShortestPath
	for r != nil {
		ret = append(ret, r)
		r = r.Prev
	}

	// Reverse.
	for i, j := 0, len(ret)-1; i < j; i, j = i+1, j-1 {
		ret[i], ret[j] = ret[j], ret[i]
	}
	return ret
}

// spHeap implements heap.Interface for ShortestPath, with ascending distance.
type spHeap []*ShortestPath

func (h spHeap) Len() int { return len(h) }

func (h spHeap) Less(i, j int) bool {
	return h[i].Distance < h[j].Distance
}

func (h spHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *spHeap) Push(x interface{}) {
	item := x.(*ShortestPath)
	*h = append(*h, item)
}

func (h *spHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*h = old[0 : n-1]
	return item
}
