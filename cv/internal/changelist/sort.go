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

package changelist

import "sort"

// Sort sorts slice of CLs by their CLID.
func Sort(cls []*CL) {
	sort.Sort(sortableCLs(cls))
}

type sortableCLs []*CL

// Len is the number of elements in the collection.
func (s sortableCLs) Len() int {
	return len(s)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (s sortableCLs) Less(i int, j int) bool {
	return s[i].ID < s[j].ID
}

// Swap swaps the elements with indexes i and j.
func (s sortableCLs) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}
