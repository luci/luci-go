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

package run

// IDs is a convenience type to faciliate handling of run IDs.
type IDs []ID

// sort.Interface copy-pasta.
func (ids IDs) Less(i, j int) bool { return ids[i] < ids[j] }
func (ids IDs) Len() int           { return len(ids) }
func (ids IDs) Swap(i, j int)      { ids[i], ids[j] = ids[j], ids[i] }

// WithoutSorted returns a subsequence of IDs without excluded IDs.
//
// Both this and the excluded slices must be sorted.
//
// If this and excluded IDs are disjoint, return this slice.
// Otherwise, returns a copy without excluded IDs.
func (ids IDs) WithoutSorted(exclude IDs) IDs {
	remaining := ids
	ret := ids
	mutated := false
	for {
		switch {
		case len(remaining) == 0:
			return ret
		case len(exclude) == 0:
			if mutated {
				ret = append(ret, remaining...)
			}
			return ret
		case remaining[0] < exclude[0]:
			if mutated {
				ret = append(ret, remaining[0])
			}
			remaining = remaining[1:]
		case remaining[0] > exclude[0]:
			exclude = exclude[1:]
		default:
			if !mutated {
				// Must copy all IDs that were skipped.
				mutated = true
				n := len(ids) - len(remaining)
				ret = make(IDs, n, len(ids)-1)
				copy(ret, ids) // copies len(ret) == n elements.
			}
			remaining = remaining[1:]
			exclude = exclude[1:]
		}
	}
}

// MakeIDs returns IDs from list of strings.
func MakeIDs(ids ...string) IDs {
	ret := make(IDs, len(ids))
	for i, id := range ids {
		ret[i] = ID(id)
	}
	return ret
}
