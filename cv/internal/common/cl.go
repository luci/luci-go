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
func (ids CLIDs) Set() CLIDsSet {
	if len(ids) == 0 {
		return nil
	}
	ret := make(CLIDsSet, len(ids))
	for _, id := range ids {
		ret.Add(id)
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

// CLIDsSet is convenience type to reduce the boilerplate.
type CLIDsSet map[CLID]struct{}

// MakeCLIDsSet returns new CLIDsSet from list of clids in int64.
func MakeCLIDsSet(ids ...int64) CLIDsSet {
	if len(ids) == 0 {
		return nil
	}
	ret := make(CLIDsSet, len(ids))
	for _, id := range ids {
		ret.AddI64(id)
	}
	return ret
}

// Reset resets the set to contain just the given IDs.
func (s CLIDsSet) Reset(ids ...CLID) {
	for id := range s {
		delete(s, CLID(id))
	}
	for _, id := range ids {
		s.Add(id)
	}
}
func (s CLIDsSet) Add(clid CLID) {
	s[clid] = struct{}{}
}
func (s CLIDsSet) Has(clid CLID) bool {
	_, exists := s[clid]
	return exists
}
func (s CLIDsSet) Del(id CLID) {
	delete(s, id)
}

func (s CLIDsSet) AddI64(id int64)      { s.Add(CLID(id)) }
func (s CLIDsSet) HasI64(id int64) bool { return s.Has(CLID(id)) }
func (s CLIDsSet) DelI64(id int64)      { s.Del(CLID(id)) }
func (s CLIDsSet) ResetI64(ids ...int64) {
	for id := range s {
		delete(s, CLID(id))
	}
	for _, id := range ids {
		s.AddI64(id)
	}
}
