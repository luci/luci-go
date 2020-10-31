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

import (
	"container/heap"

	"go.chromium.org/luci/common/errors"
)

// Query retrieves nodes in the order from closest to furthest,
// relative to roots.
type Query struct {
	Roots []Node
	// TODO(crbug.com/1136280): add Backwards
	// TODO(crbug.com/1136280): add MaxDistance
	// TODO(crbug.com/1136280): add SilbingDistance
}

// Result is one entry in the query result set.
type Result struct {
	Prev     *Result
	Node     Node
	Distance float64
}

// ErrStop when returned by a callback indicates that an iteration must stop.
var ErrStop = errors.New("stop the iteration")

// Run calls the callback for each node reachable from any of the roots.
// The nodes are reported in the ascending distance order relative to the roots,
// starting from the roots themselves.
//
// If the callback returns ErrStop, then the iteration stops.
func (q *Query) Run(g Graph, callback func(result *Result) error) error {
	// This function implements Dijstra's algorithm.

	// A min-heap of nodes ordered by distance.
	h := resultHeap{}

	// A hash map, where
	// - the key is a node
	// - the value is a shortest distance from any of the roots to the node.
	//   May shrink as the algorithm runs.
	dist := map[Node]float64{}

	// Add all roots to h and dist.
	for _, n := range q.Roots {
		if n == nil {
			return errors.Reason("one of the roots is nil").Err()
		}
		if _, ok := dist[n]; !ok {
			h = append(h, &Result{Node: n})
			dist[n] = 0
		}
	}
	heap.Init(&h)

	// A set of nodes that have been reported via callback.
	// This is needed because we might add the same node to the heap multiple
	// times when a shorter distance is discovered.
	reported := map[Node]struct{}{}

	for len(h) > 0 {
		cur := heap.Pop(&h).(*Result)

		if _, ok := reported[cur.Node]; ok {
			continue
		}
		reported[cur.Node] = struct{}{}
		if err := callback(cur); err != nil {
			if err == ErrStop {
				err = nil
			}
			return err
		}

		consider := func(other Node, distFromCur float64) {
			newDist := cur.Distance + distFromCur
			if curDist, ok := dist[other]; !ok || newDist < curDist {
				dist[other] = newDist
				heap.Push(&h, &Result{
					Prev:     cur,
					Node:     other,
					Distance: newDist,
				})
			}
		}

		cur.Node.Adjacent(func(adj Node, relRel float64) error {
			consider(adj, relRel)
			return nil
		})

		// TODO(crbug.com/1136280): use q.SiblingDistance and call consider()
	}
	return nil
}

// A resultHeap implements heap.Interface and holds Results.
type resultHeap []*Result

func (h resultHeap) Len() int { return len(h) }

func (h resultHeap) Less(i, j int) bool {
	return h[i].Distance > h[j].Distance
}

func (h resultHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *resultHeap) Push(x interface{}) {
	item := x.(*Result)
	*h = append(*h, item)
}

func (h *resultHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*h = old[0 : n-1]
	return item
}
