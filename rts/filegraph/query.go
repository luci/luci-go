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

// Query finds shortest paths to other nodes, producing a shortest-path tree
// (https://en.wikipedia.org/wiki/Shortest-path_tree).
//
// It is based on Dijkstra's algorithm
// (https://en.wikipedia.org/wiki/Dijkstra's_algorithm).
type Query struct {
	// Sources are the nodes to start from.
	Sources []Node

	// TODO(crbug.com/1136280): add Backwards
	// TODO(crbug.com/1136280): add MaxDistance
	// TODO(crbug.com/1136280): add SilbingDistance
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
// The nodes are reported in the ascending distance order relative to the
// sources, starting from the sources themselves.
//
// If the callback returns false, then the iteration stops.
func (q *Query) Run(callback func(*ShortestPath) bool) {
	// This function implements Dijstra's algorithm.

	// A min-heap of nodes ordered by distance.
	h := spHeap{}

	// A hash map, where
	// - the key is a node
	// - the value is a shortest distance from any of the sources to the node.
	//   May shrink as the algorithm runs.
	dist := map[Node]float64{}

	// Add all sources to h and dist.
	for _, n := range q.Sources {
		if n == nil {
			panic("one of the sources is nil")
		}
		if _, ok := dist[n]; !ok {
			h = append(h, &ShortestPath{Node: n})
			dist[n] = 0
		}
	}
	heap.Init(&h)

	// A set of nodes that have been reported via callback.
	// This is needed because we might add the same node to the heap multiple
	// times when a shorter distance is discovered.
	reported := map[Node]struct{}{}

	for len(h) > 0 {
		cur := heap.Pop(&h).(*ShortestPath)

		if _, ok := reported[cur.Node]; ok {
			continue
		}
		reported[cur.Node] = struct{}{}
		if !callback(cur) {
			return
		}

		consider := func(other Node, distFromCur float64) {
			newDist := cur.Distance + distFromCur
			if curDist, ok := dist[other]; !ok || newDist < curDist {
				dist[other] = newDist
				heap.Push(&h, &ShortestPath{
					Prev:     cur,
					Node:     other,
					Distance: newDist,
				})
			}
		}

		cur.Node.Adjacent(func(adj Node, relRel float64) bool {
			consider(adj, relRel)
			return true
		})

		// TODO(crbug.com/1136280): use q.SiblingDistance and call consider()
	}
}

// A resultHeap implements heap.Interface and holds Results.
type spHeap []*ShortestPath

func (h spHeap) Len() int { return len(h) }

func (h spHeap) Less(i, j int) bool {
	return h[i].Distance > h[j].Distance
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
