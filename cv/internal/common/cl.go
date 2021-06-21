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

import (
	"sort"
)

// CLID is a unique ID of a CL used internally in CV.
//
// It's just 8 bytes long and is thus much shorter than ExternalID,
// which reduces CPU & RAM & storage costs of CL graphs for multi-CL Runs.
type CLID int64

// CLIDsAsInt64s returns proto representation of CLIDs.
func CLIDsAsInt64s(ids []CLID) []int64 {
	r := make([]int64, len(ids))
	for i, id := range ids {
		r[i] = int64(id)
	}
	return r
}

// CLIDs is a convenience type to facilitate handling of a slice of CLID.
type CLIDs []CLID

// Dedupe removes duplicates in place and sorts the slice.
//
// Note: Does not preserve original order.
func (p *CLIDs) Dedupe() {
	clids := *p
	if len(clids) <= 1 {
		return
	}
	sort.Sort(clids)
	n, prev, skipped := 0, clids[0], false
	for _, id := range clids[1:] {
		if id == prev {
			skipped = true
			continue
		}
		n++
		if skipped {
			clids[n] = id
		}
		prev = id
	}
	*p = clids[:n+1]
}

// Len is the number of elements in the collection.
func (ids CLIDs) Len() int {
	return len(ids)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (ids CLIDs) Less(i int, j int) bool {
	return ids[i] < ids[j]
}

// Swap swaps the elements with indexes i and j.
func (ids CLIDs) Swap(i int, j int) {
	ids[i], ids[j] = ids[j], ids[i]
}

// Set returns a new set of CLIDs.
func (ids CLIDs) Set() map[CLID]struct{} {
	if ids == nil {
		return nil
	}
	ret := make(map[CLID]struct{}, len(ids))
	for _, id := range ids {
		ret[id] = struct{}{}
	}
	return ret
}

// Contains returns true if CLID is inside these CLIDs.
func (ids CLIDs) Contains(id CLID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// MakeCLIDs returns CLIDs from list of clids in int64.
func MakeCLIDs(ids ...int64) CLIDs {
	if ids == nil {
		return nil
	}
	ret := make(CLIDs, len(ids))
	for i, id := range ids {
		ret[i] = CLID(id)
	}
	return ret
}
