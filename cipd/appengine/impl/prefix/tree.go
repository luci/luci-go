// Copyright 2026 The LUCI Authors.
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

// Package prefix implements a segment-oriented radix tree for CIPD prefixes.
package prefix

import (
	"cmp"
	"iter"
	"slices"
	"strings"
)

// Tree is an immutable radix tree which relies on '/'-separated paths.
//
// This is necessary because we want to be able to efficiently iterate over
// multiple trees simultaneously in order to support ListPrefix:
//   - Iteration over global prefixes.cfg to identify when project ownership
//     changes in the CIPD package hierarchy.
//   - Iteration over the currently selected project config to identify when
//     the assigned realm changes.
//   - Iteration over an extracted version of the DB ACLs to see when a prefix
//     becomes visible due to the caller having READER and the global config
//     allowing legacy ACL fallback.
//
// In particular, this walk process should only yield when some bit of this
// implied configuration changes.
//
// We could use go-radix for this, but it would effectively require building a
// merged tree on every list operation, and I don't want to.
//
// In the steady state (with no config changes), walking the combined tree
// is a zero-allocation activity.
//
// NOTE: There is an opportunity to 'optimize' this further by using a smaller
// index type than pointers (e.g. uint32), but it makes the code gnarlier.
type Tree[V any] struct {
	// nodes holds all nodes in the tree.
	// nodes[0] is the root, and every parent precedes all of its children.
	nodes []node[V]
}

type node[V any] struct {
	// prefix is the normalized full path from the root to this node ("" for the
	// root node, e.g. "a/b/c/" with trailing '/' for non-root nodes).
	//
	// Non-root internal nodes slice their prefix out of a descendant leaf's
	// prefix so non-leaf strings are not retained in memory.
	prefix string

	// val is the effective value at this node (valid iff hasVal is true).
	// If hasVal is false, len(children) > 1.
	val    V
	hasVal bool

	// children are sorted in byte order of their first slash-terminated segment
	// after `prefix` (child.prefix[len(prefix):]), subsliced out of Tree.nodes.
	children []node[V]
}

// MergeFn is called by [New] in top-down order on a pair of (parent, child)
// values, where `parent` is the already-merged effective value of the nearest
// ancestor that has a value, and `child` is value yielded by the iterator
// given to [New] for the child node.
//
// Note that if the iterator yields multiple values for the same path, then
// only the last value is kept (and thus only the last value is given to
// [MergeFn]).
//
// It should return the effective `V` to save on the child node.
//
// Because `parent` is retained on the ancestor node and shared across sibling
// subtrees, `MergeFn` must not mutate `parent`. Mutating `child` in-place is
// valid when each yielded `child` value is distinct.
//
// A `nil` MergeFn bypasses all merge functionality (parent and child values are
// left alone).
type MergeFn[V any] func(parent, child V) V

type rawEntry[V any] struct {
	path string
	val  V
}

// makeRawEntries ingests all (path, value)s from the iter, returning them
// normalized ("" for root, trailing '/' for non-root), sorted in byte order,
// and deduplicated (keeping the last value yielded for duplicate normalized
// paths).
//
// A root entry is guaranteed at index 0.
//
// This also returns the exact number of radix tree nodes (leafs and
// intermediates) required to represent the returned entries.
func makeRawEntries[V any](defaultRoot V, n int, values iter.Seq2[string, V]) (dedup []rawEntry[V], nodeCount int, hasExplicitRoot bool) {
	entries := make([]rawEntry[V], 0, max(0, n)+1)
	for path, value := range values {
		clean := normPath(path)
		hasExplicitRoot = hasExplicitRoot || clean == ""
		entries = append(entries, rawEntry[V]{clean, value})
	}
	if !hasExplicitRoot {
		entries = append(entries, rawEntry[V]{"", defaultRoot})
	}

	slices.SortStableFunc(entries, func(a, b rawEntry[V]) int {
		return cmp.Compare(a.path, b.path)
	})

	dedup = entries[:1] // entries[0] is guaranteed to be root ("")
	nodeCount = 1
	// depths is a stack of byte lengths for all nodes along prev.path that may
	// still gain children from subsequent entries.
	depths := []int{0}
	for _, e := range entries[1:] {
		prev := &dedup[len(dedup)-1]
		if prev.path == e.path {
			// It's a duplicate - overwrite `prev` with e and continue.
			*prev = e
			continue
		}

		pfxLen := commonPrefixLen(prev.path, e.path)
		dedup = append(dedup, e)

		// Pop depths until it has a value less than the common prefix len (i.e.
		// this new entry goes under a different, earlier, subtree).
		for depths[len(depths)-1] > pfxLen {
			depths = depths[:len(depths)-1]
		}
		// If the current depth isn't *exactly* pfxLen, we will need an intermediate
		// (value-less) node here.
		if depths[len(depths)-1] < pfxLen {
			depths = append(depths, pfxLen)
			nodeCount++
		}
		// Finally, every entry (with a value) definitely needs a node.
		depths = append(depths, len(e.path))
		nodeCount++
	}
	return dedup, nodeCount, hasExplicitRoot
}

// New constructs an immutable segment-oriented radix [Tree] from `values`.
//
// `defaultRoot` is used as the root entry value if `values` does not yield a
// root path (`""` or `"/"`). If `values` DOES yield a root path, `defaultRoot`
// will be used as the `parent` for `merge` on the yielded root value.
//
// `n` is a size hint for pre-allocating internal buffers.
// If duplicate normalized paths are yielded by `values`, the last value wins.
func New[V any](defaultRoot V, n int, values iter.Seq2[string, V], merge MergeFn[V]) *Tree[V] {
	entries, nodeCount, hasExplicitRoot := makeRawEntries(defaultRoot, n, values)
	nodes := make([]node[V], nodeCount)
	alloc := nodes[1:]
	buildNode(&nodes[0], entries, 0, defaultRoot, hasExplicitRoot, merge, &alloc)
	return &Tree[V]{nodes: nodes}
}

// buildNode populates `dst` from pre-sorted `entries` and carves child slices
// out of `alloc`.
//
// `depth` is the byte length of `dst`'s slash-terminated prefix (0 for root).
//
// `mergeSelf` controls if we should call `merge` on the value for *this* node.
// It is only ever false for the root of the whole tree if `values` did not
// explicitly yield a root node.
func buildNode[V any](dst *node[V], entries []rawEntry[V], depth int, parentVal V, mergeSelf bool, merge MergeFn[V], alloc *[]node[V]) {
	if depth > 0 {
		// Because entries contains all descendants of this node in sorted prefix
		// order, entries[len(entries)-1] has no descendants and is guaranteed to
		// become a leaf node. Slicing its path retains only leaf strings on the
		// heap, resulting in less resident garbage.
		dst.prefix = entries[len(entries)-1].path[:depth]
	}
	effectiveVal := parentVal

	// If the top of entries exactly matches our depth in the tree, then this
	// node has a value.
	//
	// This can be false for intermediate nodes which just contain branches for
	// children (in this case entries[0].path will be deeper than our current
	// depth).
	if len(entries[0].path) == depth {
		dst.val = entries[0].val
		if mergeSelf && merge != nil {
			dst.val = merge(parentVal, dst.val)
		}
		dst.hasVal = true
		effectiveVal = dst.val
		entries = entries[1:]
	}

	if len(entries) == 0 {
		return
	}

	// Count immediate children so we can carve a contiguous []node[V] subslice
	// out of `alloc` before recursing into descendants.
	numChildren := 0
	for rem := entries; len(rem) > 0; {
		firstSeg, _ := cutSlashSegment(rem[0].path[depth:])
		hi := 1
		for hi < len(rem) && strings.HasPrefix(rem[hi].path[depth:], firstSeg) {
			hi++
		}
		numChildren++
		rem = rem[hi:]
	}

	// Install these nodes as our children slice and remove them from alloc.
	dst.children = (*alloc)[:numChildren:numChildren]
	*alloc = (*alloc)[numChildren:]

	for childIdx := range numChildren {
		firstSeg, _ := cutSlashSegment(entries[0].path[depth:])
		hi := 1
		for hi < len(entries) && strings.HasPrefix(entries[hi].path[depth:], firstSeg) {
			hi++
		}

		// Compute the common segment prefix length across [0:hi] relative to
		// depth. Because entries is sorted, comparing 0 and hi-1 suffices.
		lcp := commonPrefixLen(entries[0].path[depth:], entries[hi-1].path[depth:])

		buildNode(&dst.children[childIdx], entries[:hi], depth+lcp, effectiveVal, true, merge, alloc)
		entries = entries[hi:]
	}
}

// Get returns the effective value for `path`, which is the value associated
// with `path` itself, or the nearest ancestor which has an associated value.
//
// Guaranteed to return *some* value for any non-nil Tree returned by [New]
// (because all trees are constructed with a value for the root). Calling Get
// on a nil or zero-value Tree returns the zero value of V.
func (t *Tree[V]) Get(path string) V {
	if t == nil || len(t.nodes) == 0 {
		var zero V
		return zero
	}

	cur := &t.nodes[0]
	bestVal := cur.val

	rest := path
	for {
		seg, nextRest, ok := nextSegment(rest)
		if !ok {
			return bestVal
		}
		rest = nextRest

		parentLen := len(cur.prefix)
		idx, found := slices.BinarySearchFunc(cur.children, seg, func(n node[V], target string) int {
			return compareFirstSegment(n.prefix[parentLen:], target)
		})
		if !found {
			return bestVal
		}
		child := &cur.children[idx]

		_, edgeRest := cutSlashSegment(child.prefix[parentLen:])
		for edgeRest != "" {
			var want string
			want, edgeRest = cutSlashSegment(edgeRest)
			got, r, hasMore := nextSegment(rest)
			if !hasMore || got != want[:len(want)-1] {
				return bestVal
			}
			rest = r
		}

		cur = child
		if cur.hasVal {
			bestVal = cur.val
		}
	}
}
