// Copyright 2025 The LUCI Authors.
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

// Package indexset implements a set for indices.
package indexset

import (
	"sort"
)

// Set is the base type implemented by this package.
type Set map[uint32]struct{}

// New returns a new empty set, with capacity sizeHint.
func New(sizeHint int) Set {
	return make(Set, sizeHint)
}

// NewFromSlice returns a new set, initialized with the unique values in the
// given slice.
func NewFromSlice(values ...uint32) Set {
	result := New(len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

// Has returns whether this set contains the given value.
func (s Set) Has(value uint32) bool {
	_, has := s[value]
	return has
}

// HasAll returns whether this set contains all the values in the given slice.
func (s Set) HasAll(values ...uint32) bool {
	for _, value := range values {
		if !s.Has(value) {
			return false
		}
	}
	return true
}

// Add ensures this set contains the given value.
// Returns true if it was added, and false if it already contained the value.
func (s Set) Add(value uint32) bool {
	if s.Has(value) {
		return false
	}
	s[value] = struct{}{}
	return true
}

// AddAll ensures this set contains all the values in the given slice.
func (s Set) AddAll(values []uint32) {
	for _, value := range values {
		s[value] = struct{}{}
	}
}

// Remove ensures this set does not contain the value.
// Returns true if the value was deleted, and false if the value was already not
// in the set.
func (s Set) Remove(value uint32) bool {
	if !s.Has(value) {
		return false
	}
	delete(s, value)
	return true
}

// RemoveAll ensures this set does not contain any of the values in the given
// slice.
func (s Set) RemoveAll(values []uint32) {
	for _, value := range values {
		delete(s, value)
	}
}

// Len returns the number of elements in this set.
func (s Set) Len() int {
	return len(s)
}

// ToSlice renders this set to a slice of all values contained in the set.
func (s Set) ToSlice() []uint32 {
	result := make([]uint32, 0, len(s))
	for value := range s {
		result = append(result, value)
	}
	return result
}

// ToSortedSlice renders this set to a sorted slice of all values contained in
// the set.
func (s Set) ToSortedSlice() []uint32 {
	result := s.ToSlice()
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// Intersect returns a new set which is the intersection of this set with
// the other set.
func (s Set) Intersect(other Set) Set {
	big := s
	small := other
	if len(s) < len(other) {
		small, big = s, other
	}
	result := make(Set)
	for k := range small {
		if big.Has(k) {
			result[k] = struct{}{}
		}
	}
	return result
}

// Difference returns a new set which is this set with all elements from
// the other set removed (i.e. `self - other`).
func (s Set) Difference(other Set) Set {
	result := make(Set)
	for k := range s {
		if !other.Has(k) {
			result[k] = struct{}{}
		}
	}
	return result
}

// Union returns a new set which contains all element from this set, as
// well as all elements from the other set.
func (s Set) Union(other Set) Set {
	result := New(len(s))
	for k := range s {
		result[k] = struct{}{}
	}
	for k := range other {
		result[k] = struct{}{}
	}
	return result
}

// Contains returns true if this set contains all elements from the other set.
func (s Set) Contains(other Set) bool {
	for k := range other {
		if !s.Has(k) {
			return false
		}
	}
	return true
}
