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
	"fmt"
	"math"

	"go.chromium.org/luci/common/errors"
)

// Distance computes distance between two nodes based on their relevancy.
func Distance(relevancy float64) float64 {
	switch {
	case relevancy == 1:
		return 0
	case relevancy < 0 || relevancy > 1:
		panic("relevance must be between 0.0 and 1.0 inclusive")
	default:
		return -math.Log(relevancy)
	}
}

// Relevance converts distance to relevance. It is the opposite of Distance().
func Relevance(distance float64) float64 {
	switch {
	case distance == 0:
		return 1
	case distance < 0:
		panic("distance cannot be negative")
	default:
		return math.Pow(math.E, -distance)
	}
}

const siblingRelevancy = 0.5

// Query is a request to retreive nodes. See Graph.Query().
type Query struct {
	Roots            []*Node
	Backwards        bool
	MinRelevance     float64
	SiblingRelevance float64
}

// ResultItem is one entry in the query result set.
type ResultItem struct {
	Prev      *ResultItem
	Node      *Node
	Relevance float64
}

// Path reconstructs the path from the traversal root to the node in r.
func (r *ResultItem) Path() []*ResultItem {
	var ret []*ResultItem
	for r != nil {
		ret = append(ret, r)
		r = r.Prev
	}

	// Reverse
	for i, j := 0, len(ret)-1; i < j; i, j = i+1, j-1 {
		ret[i], ret[j] = ret[j], ret[i]
	}
	return ret
}

// Query retrieves nodes in the order from distance from the given roots.
func (g *Graph) Query(q Query, fn func(result *ResultItem) error) error {
	h := resultHeap{}
	inHeap := map[*Node]float64{}

	for _, n := range q.Roots {
		if n == nil {
			return errors.Reason("one of the roots is nil").Err()
		}
		if _, ok := inHeap[n]; !ok {
			h = append(h, &ResultItem{Node: n, Relevance: 1})
			inHeap[n] = 1
		}
	}
	heap.Init(&h)

	returned := map[*Node]struct{}{}
	for len(h) > 0 {
		cur := heap.Pop(&h).(*ResultItem)

		if _, ok := returned[cur.Node]; ok {
			continue
		}
		returned[cur.Node] = struct{}{}
		if err := fn(cur); err != nil {
			if err == ErrStop {
				err = nil
			}
			return err
		}

		consider := func(other *Node, relativeRelevancy float64) {
			switch newRel := cur.Relevance * relativeRelevancy; {
			case other == nil:
				panic("other is nil")
			case math.IsInf(newRel, 0):
				panic(fmt.Sprintf("newRel is inf for %q; relativeRelevancy is %f", other.Path, relativeRelevancy))
			case newRel > 1:
				panic(fmt.Sprintf("newRel %f > 1, with relativeRelevancy %f", newRel, relativeRelevancy))
			case newRel <= inHeap[other]:
				return
			case newRel < q.MinRelevance:
				return
			default:
				inHeap[other] = newRel
				heap.Push(&h, &ResultItem{
					Prev:      cur,
					Node:      other,
					Relevance: newRel,
				})
			}
		}

		if cur.Node.weight != 0 {
			for _, a := range cur.Node.adjacent {
				var pair nodePair
				if q.Backwards {
					pair = nodePair{a, cur.Node}
				} else {
					pair = nodePair{cur.Node, a}
				}

				if !a.IsTreeLeaf() {
					panic(fmt.Sprintf("%q is adjacent, but it is not a lead", a.Path))
				}
				if pair.from.weight == 0 {
					// TODO(nodir): verify this
					continue
				}

				consider(a, g.weight[pair]/pair.from.weight)
			}
		}

		if q.SiblingRelevance > 0 {
			for _, c := range cur.Node.children {
				if c.IsTreeLeaf() {
					consider(c, 1.0)
				}
			}

			if len(cur.Node.Path) > 1 {
				parentPath, _ := cur.Node.Path.Split()
				parent := g.Node(parentPath)
				if parent == nil {
					panic(errors.Reason("parent of %q not found", cur.Node.Path).Err())
				}
				consider(parent, q.SiblingRelevance)
			}
		}
	}
	return nil
}

// ShortestPath returns the shortest path from one node to another.
// ResultItem.Path() can be used to reconstruct the path.
// Returns nil if the path does not exist.
func (g *Graph) ShortestPath(from, to *Node, backwards bool) (*ResultItem, error) {
	q := Query{
		Roots:     []*Node{from},
		Backwards: backwards,
	}

	var ret *ResultItem
	err := g.Query(q, func(result *ResultItem) error {
		if result.Node == to {
			ret = result
			return ErrStop
		}
		return nil
	})
	return ret, err
}

// A resultHeap implements heap.Interface and holds Items.
type resultHeap []*ResultItem

func (h resultHeap) Len() int { return len(h) }

func (h resultHeap) Less(i, j int) bool {
	return h[i].Relevance > h[j].Relevance
}

func (h resultHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *resultHeap) Push(x interface{}) {
	item := x.(*ResultItem)
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
