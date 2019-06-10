// Copyright 2017 The LUCI Authors.
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

// Package sortby provides a succinct way to generate correctly-behaved Less
// functions for use with the stdlib 'sort' package.
package sortby

// LessFn is the type of the function which compares element i with element j of
// a given slice. Unlike the stdlib sort interpretation of this function,
// a LessFn in sortby should only compare a single field in your datastructure's
// elements. Multiple LessFns can be composed with Chain to create a composite
// Less implementation to pass to sort.
type LessFn func(i, j int) bool

// Chain is a list of LessFns, each of which sorts a single aspect of your
// object. Nil LessFns will be ignored.
type Chain []LessFn

// Use is a sort-compatible LessFn that actually executes the full chain of
// comparisons.
func (c Chain) Use(i, j int) bool {
	for _, less := range c {
		if less == nil {
			continue
		}
		if less(i, j) {
			return true
		} else if less(j, i) {
			return false
		}
	}
	return false
}
