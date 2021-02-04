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
