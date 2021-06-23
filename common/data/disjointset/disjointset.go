// Copyright 2021 The LUCI Authors.
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

// Package DisjointSet provides a disjoint-set data structure with fixed size.
package disjointset

import (
	"fmt"
	"sort"
	"strings"
)

// DisjointSet implements a disjoint-set data structure with fixed size.
//
// Each element is represented by a 0-based array index.
//
// Also known as union-find or merge-find set.
// See https://en.wikipedia.org/wiki/Disjoint-set_data_structure.
type DisjointSet struct {
	items []item
	count int // how many disjoint sets remaining
}

type item struct {
	parent int // equals to itself if it is root
	size   int // only if root: number of elements in its tree (set)
}

// New constructs new DisjointSet of `size` elements each in its own set.
func New(size int) DisjointSet {
	d := DisjointSet{
		items: make([]item, size),
		count: size,
	}
	for i := range d.items {
		d.items[i] = item{i, 1}
	}
	return d
}

// Count returns number of disjoint sets.
func (d DisjointSet) Count() int {
	return d.count
}

// Disjoint returns true if i-th and j-th elements are in different sets.
func (d DisjointSet) Disjoint(i, j int) bool {
	return d.RootOf(i) != d.RootOf(j)
}

// RootOf returns root of the set with i-th element.
func (d DisjointSet) RootOf(i int) int {
	for {
		parent := d.items[i].parent
		if parent == i {
			return i
		}
		// Do so called "path-splitting" while traversing path to the root:
		// each visited node's parent is updated to its grand-parent (if exists).
		i, d.items[i].parent = parent, d.items[parent].parent
	}
}

// SizeOf returns size of the set with i-th element.
func (d DisjointSet) SizeOf(i int) int {
	root := d.RootOf(i)
	return d.items[root].size
}

// Merge merges i-th and j-th elements' sets.
//
// If they are in the same set, does nothing and returns false.
// Else, does the merges and returns true.
//
// This is the only allowed mutation.
func (d *DisjointSet) Merge(i, j int) bool {
	i, j = d.RootOf(i), d.RootOf(j)
	if i == j {
		return false
	}
	// Ensure i-th has at least as many elements as j-th.
	if d.items[i].size < d.items[j].size {
		j, i = i, j
	}
	d.items[j].parent = i // i-th is now the root of two sets
	d.items[i].size += d.items[j].size
	d.count--
	return true
}

// Sets returns disjoint sets in an unspecified unorder.
//
// Elements in each set are sorted.
func (d DisjointSet) Sets() [][]int {
	byParent := make(map[int][]int, d.count)
	for i := range d.items {
		root := d.RootOf(i)
		byParent[root] = append(byParent[root], i)
	}
	out := make([][]int, 0, d.count)
	for _, set := range byParent {
		sort.Ints(set)
		out = append(out, set)
	}
	return out
}

// SortedSets returns disjoint sets ordered by size, largest first.
//
// For determinism, when size is the same, set with smaller first element is
// first.
//
// Elements in each set are sorted.
func (d DisjointSet) SortedSets() [][]int {
	out := d.Sets()
	sort.Slice(out, func(i, j int) bool {
		switch l, r := len(out[i]), len(out[j]); {
		case l > r:
			return true
		case l < r:
			return false
		default:
			return out[i][0] < out[j][0]
		}
	})
	return out
}

// String formats a disjoint set as a human-readable string.
func (d DisjointSet) String() string {
	sb := strings.Builder{}
	sb.WriteString("DisjointSet([\n")
	for _, set := range d.SortedSets() {
		sb.WriteString("  [")
		for i, el := range set {
			if i != 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%d", el)
		}
		sb.WriteString("]\n")
	}
	sb.WriteString("])")
	return sb.String()
}
