// Copyright 2015 The LUCI Authors.
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

package stringset

import "sort"

// Set is the base type. make(Set) can be used too.
type Set map[string]struct{}

// New returns a new string Set implementation.
func New(sizeHint int) Set {
	return make(Set, sizeHint)
}

// NewFromSlice returns a new string Set implementation,
// initialized with the values in the provided slice.
func NewFromSlice(vals ...string) Set {
	ret := make(Set, len(vals))
	for _, k := range vals {
		ret[k] = struct{}{}
	}
	return ret
}

// Has returns true iff the Set contains value.
func (s Set) Has(value string) bool {
	_, ret := s[value]
	return ret
}

// HasAll returns true iff the Set contains all the given values.
func (s Set) HasAll(values ...string) bool {
	for _, v := range values {
		if !s.Has(v) {
			return false
		}
	}
	return true
}

// Add ensures that Set contains value, and returns true if it was added (i.e.
// it returns false if the Set already contained the value).
func (s Set) Add(value string) bool {
	if _, ok := s[value]; ok {
		return false
	}
	s[value] = struct{}{}
	return true
}

// AddAll ensures that Set contains all values.
func (s Set) AddAll(values []string) {
	for _, value := range values {
		s[value] = struct{}{}
	}
}

// Del removes value from the set, and returns true if it was deleted (i.e. it
// returns false if the Set did not already contain the value).
func (s Set) Del(value string) bool {
	if _, ok := s[value]; !ok {
		return false
	}
	delete(s, value)
	return true
}

// DelAll ensures that Set contains none of values.
func (s Set) DelAll(values []string) {
	for _, value := range values {
		delete(s, value)
	}
}

// Peek returns an arbitrary element from the set. If the set was empty, this
// returns ("", false).
func (s Set) Peek() (string, bool) {
	for k := range s {
		return k, true
	}
	return "", false
}

// Pop removes and returns an arbitrary element from the set and removes it from the
// set. If the set was empty, this returns ("", false).
func (s Set) Pop() (string, bool) {
	for k := range s {
		delete(s, k)
		return k, true
	}
	return "", false
}

// Iter calls `cb` for each item in the set. If `cb` returns false, the
// iteration stops.
func (s Set) Iter(cb func(string) bool) {
	for k := range s {
		if !cb(k) {
			break
		}
	}
}

// Len returns the number of items in this set.
func (s Set) Len() int {
	return len(s)
}

// Dup returns a duplicate set.
func (s Set) Dup() Set {
	ret := make(Set, len(s))
	for k := range s {
		ret[k] = struct{}{}
	}
	return ret
}

// ToSlice renders this set to a slice of all values.
func (s Set) ToSlice() []string {
	ret := make([]string, 0, len(s))
	for k := range s {
		ret = append(ret, k)
	}
	return ret
}

// ToSortedSlice renders this set to a sorted slice of all values, ascending.
func (s Set) ToSortedSlice() []string {
	ret := s.ToSlice()
	sort.Strings(ret)
	return ret
}

// Intersect returns a new Set which is the intersection of this set with the
// other set.
func (s Set) Intersect(other Set) Set {
	smallLen := len(s)
	if lo := len(other); lo < smallLen {
		smallLen = lo
	}
	ret := make(Set, smallLen)
	for k := range s {
		if _, ok := other[k]; ok {
			ret[k] = struct{}{}
		}
	}
	return ret
}

// Difference returns a new Set which is this set with all elements from other
// removed (i.e. `self - other`).
func (s Set) Difference(other Set) Set {
	ret := make(Set)
	for k := range s {
		if _, ok := other[k]; !ok {
			ret[k] = struct{}{}
		}
	}
	return ret
}

// Union returns a new Set which contains all element from this set, as well
// as all elements from the other set.
func (s Set) Union(other Set) Set {
	ret := make(Set, len(s))
	for k := range s {
		ret[k] = struct{}{}
	}
	for k := range other {
		ret[k] = struct{}{}
	}
	return ret
}

// Contains returns true iff the given set contains all elements from the other set.
func (s Set) Contains(other Set) bool {
	for k := range other {
		if !s.Has(k) {
			return false
		}
	}
	return true
}
