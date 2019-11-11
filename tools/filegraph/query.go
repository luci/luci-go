package main

import (
	"container/heap"
	"fmt"
	"math"

	"go.chromium.org/luci/common/errors"
)

func Distance(relevancy float64) float64 {
	if relevancy == 1 {
		return 0
	}
	dist := -math.Log(relevancy)
	if dist < 0 {
		panic(fmt.Sprintf("negative distance with relevancy %f", relevancy))
	}
	return dist
}

const siblingRelevancy = 0.7

type query struct {
	roots     []*node
	backwards bool
}

type queryResult struct {
	prev      *queryResult
	n         *node
	relevance float64
}

func (r *queryResult) path() []*queryResult {
	var ret []*queryResult
	for r != nil {
		ret = append(ret, r)
		r = r.prev
	}

	// Reverse
	for i, j := 0, len(ret)-1; i < j; i, j = i+1, j-1 {
		ret[i], ret[j] = ret[j], ret[i]
	}
	return ret
}

func (g *graph) Query(q query, fn func(result *queryResult) error) error {
	h := resultHeap{}
	inHeap := map[*node]float64{}

	for _, n := range q.roots {
		if n == nil {
			return errors.Reason("one of the roots is nil").Err()
		}
		h = append(h, &queryResult{n: n, relevance: 1})
		inHeap[n] = 1
	}
	heap.Init(&h)

	returned := map[*node]struct{}{}
	for len(h) > 0 {
		cur := heap.Pop(&h).(*queryResult)

		if _, ok := returned[cur.n]; ok {
			continue
		}
		returned[cur.n] = struct{}{}
		if err := fn(cur); err != nil {
			if err == errStop {
				err = nil
			}
			return err
		}

		consider := func(other *node, relativeRelevancy float64) {
			if other == nil {
				panic("other is nil")
			}

			newRel := cur.relevance * relativeRelevancy
			if math.IsInf(newRel, 0) {
				panic(fmt.Sprintf("newRel is inf for %q; relativeRelevancy is %f", other.path, relativeRelevancy))
			}
			if newRel > 1 {
				panic(fmt.Sprintf("newRel %f > 1, with relativeRelevancy %f", newRel, relativeRelevancy))
			}

			if newRel <= inHeap[other] {
				return
			}
			inHeap[other] = newRel
			heap.Push(&h, &queryResult{
				prev:      cur,
				n:         other,
				relevance: newRel,
			})
		}

		for _, a := range cur.n.adjacent {
			var pair nodePair
			if q.backwards {
				pair = nodePair{a, cur.n}
			} else {
				pair = nodePair{cur.n, a}
			}

			if !a.isTreeLeaf() {
				panic(fmt.Sprintf("%q is adjacent, but it is not a lead", a.path))
			}
			if pair.from.weight == 0 {
				// TODO(nodir): verify this
				continue
			}

			consider(a, g.weight[pair]/pair.from.weight)
		}

		for _, c := range cur.n.children {
			if c.isTreeLeaf() {
				consider(c, 1)
			}
		}

		if len(cur.n.path) > 1 {
			parentPath, _ := cur.n.path.Split()
			parent := g.getNode(parentPath)
			if parent == nil {
				panic(errors.Reason("parent of %q not found", cur.n.path).Err())
			}
			consider(parent, siblingRelevancy)
		}
	}
	return nil
}

func (g *graph) shortestPath(from, to *node, backwards bool) (*queryResult, error) {
	q := query{
		roots:     []*node{from},
		backwards: backwards,
	}

	var ret *queryResult
	err := g.Query(q, func(result *queryResult) error {
		if result.n == to {
			ret = result
			return errStop
		}
		return nil
	})
	return ret, err
}

// A resultHeap implements heap.Interface and holds Items.
type resultHeap []*queryResult

func (h resultHeap) Len() int { return len(h) }

func (h resultHeap) Less(i, j int) bool {
	return h[i].relevance > h[j].relevance
}

func (h resultHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *resultHeap) Push(x interface{}) {
	item := x.(*queryResult)
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
