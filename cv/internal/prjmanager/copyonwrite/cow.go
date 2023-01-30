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

// Package copyonwrite providers helpers for modifying slices in Copy-on-Write way.
package copyonwrite

import (
	"fmt"
	"sort"
)

// Slice abstracts out copy-on-write slices of pointers to objects.
type Slice interface {
	Len() int
	At(index int) interface{}
	Append(v interface{}) Slice
	// CloneShallow returns new slice of the given capacity with elements copied
	// from [:length] of this slice.
	CloneShallow(length, capacity int) Slice
}

// SortedSlice is a sorted Slice.
type SortedSlice interface {
	Slice
	sort.Interface
	// LessElements returns true if element `a` must be before `b`.
	//
	// Unlike sort.Interface's Less, this compares arbitrary elements.
	//
	// Why not use sort.Interface's Less?
	// Because during merge phase of (newly created, updated) slices, it's
	// necessary to compare elements from different slices. OK, OK, it's possible
	// to move both elements to one slice and re-use Less. For example, a temp
	// slice of 2 elements can be always allocated for this very purpose, or one
	// can smartly re-use resulting slice for this. However, either way it's
	// slower and more complicated than requiring boilerplate in each SortedSlice
	// type, which can be re-used in Less() implementation.
	LessElements(a, b interface{}) bool
}

// Deletion is special return value for Modifier to imply deletion.
var Deletion = deletion{}

type deletion struct{}

// Modifier takes a pointer to an existing COW object and returns:
//   - special DeleteMe, if this object should be deleted from Slice;
//   - the same object, if there are no modifications;
//   - new object, if there were modifications. If working with SortedSlice,
//     the new object must have the same sorting key.
type Modifier func(interface{}) interface{}

// Update modifies existing elements and adds new ones to Slice.
//
// If given Slice implements SortedSlice, assumes and preserves its sorted
// order.
//
// COWModifier, if not nil, is called on each existing object to modify or
// possible delete it.
//
// Sorts toAdd slice if using SortedSlice.
//
// Returns (new slice, true) if it was modified.
// Returns (old slice, false) otherwise.
func Update(in Slice, modifier Modifier, toAdd Slice) (Slice, bool) {
	inLen := 0
	if in != nil {
		inLen = in.Len()
	}
	if modifier == nil {
		modifier = noopModifier
	}
	cLen := 0
	if toAdd != nil {
		cLen = toAdd.Len()
	}
	_, isSorted := in.(SortedSlice)
	if isSorted && cLen > 0 {
		sortable, ok := toAdd.(SortedSlice)
		if !ok {
			panic(fmt.Errorf("Different types for in and toAdd slices: %T vs %T", in, toAdd))
		}
		sort.Sort(sortable)
	}
	switch {
	case inLen == 0 && cLen == 0:
		return in, false
	case inLen == 0:
		return toAdd, true
	case cLen == 0:
		return modifyRemaining(in, 0, modifier, nil)
	case !isSorted:
		return modifyRemaining(in, 0, modifier, toAdd)
	}

	// Merge two sorted sequences into `out` while modifying `in` elements at the
	// same time.
	less := in.(SortedSlice).LessElements
	out := in.CloneShallow(0, inLen+cLen)
	i, c := 0, 0
	for {
		if c == cLen {
			return modifyRemaining(in, i, modifier, out)
		}
		if i == inLen {
			for ; c < cLen; c++ {
				out = out.Append(toAdd.At(c))
			}
			return out, true
		}
		iOld := in.At(i)
		cNew := toAdd.At(c)
		if less(cNew, iOld) {
			c++
			out = out.Append(cNew)
			continue
		}
		i++
		if iNew := modifier(iOld); iNew != Deletion {
			out = out.Append(iNew)
		}
	}
}

func noopModifier(v interface{}) interface{} { return v }

func modifyRemaining(in Slice, i int, modifier Modifier, out Slice) (Slice, bool) {
	l := in.Len()
	for ; i < l; i++ {
		before := in.At(i)
		after := modifier(before)
		if out == nil && after == before {
			continue // no changes so far
		}
		if out == nil {
			out = in.CloneShallow(i, l)
		}
		if after != Deletion {
			out = out.Append(after)
		}
	}
	if out == nil {
		return in, false
	}
	return out, true
}
