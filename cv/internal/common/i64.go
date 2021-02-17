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

package common

import "sort"

// UniqueSorted sorts & removes duplicates in place.
//
// Returns the potentially shorter slice.
func UniqueSorted(v []int64) []int64 {
	if len(v) <= 1 {
		return v
	}

	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	n, prev, skipped := 0, v[0], false
	for _, id := range v[1:] {
		if id == prev {
			skipped = true
			continue
		}
		n++
		if skipped {
			v[n] = id
			prev = id
		}
	}
	return v[:n+1]
}

// DifferenceSorted returns all int64s in the first slice and not the second.
//
// Both slices must be sorted. Doesn't modify input slices.
func DifferenceSorted(a, b []int64) []int64 {
	var diff []int64
	for {
		if len(b) == 0 {
			return append(diff, a...)
		}
		if len(a) == 0 {
			return diff
		}
		x, y := a[0], b[0]
		switch {
		case x == y:
			a, b = a[1:], b[1:]
		case x < y:
			diff = append(diff, x)
			a = a[1:]
		default:
			b = b[1:]
		}
	}
}

// UnionSorted returns sorted unique int64s from two slices.
//
// Both slices must be sorted and unique. Doesn't modify input slices.
func UnionSorted(a, b []int64) []int64 {
	u := make([]int64, 0, len(a)+len(b))
	for {
		if len(b) == 0 {
			return append(u, a...)
		}
		if len(a) == 0 {
			return append(u, b...)
		}
		x, y := a[0], b[0]
		switch {
		case x == y:
			a, b = a[1:], b[1:]
			u = append(u, x)
		case x < y:
			u = append(u, x)
			a = a[1:]
		default:
			u = append(u, y)
			b = b[1:]
		}
	}
}
