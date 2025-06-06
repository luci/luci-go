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

package datastore

import (
	"sort"

	"go.chromium.org/luci/common/errors"
)

// DroppedArgTracker is used to track dropping items from Keys as well as meta
// and/or PropertyMap arrays from one layer of the RawInterface to the next.
//
// If you're not writing a datastore backend implementation (like
// "go.chromium.org/luci/gae/impl/*"), then you can ignore this type.
//
// For example, say your GetMulti method was passed 4 arguments, but one of them
// was bad. DroppedArgTracker would allow you to "drop" the bad entry, and then
// synthesize new keys/meta/values arrays excluding the bad entry. You could
// then map from the new arrays back to the indexes of the original arrays.
//
// This DroppedArgTracker will do no allocations if you don't end up dropping
// any arguments (so in the 'good' case, there are zero allocations).
//
// Example:
//
//	  Say we're given a list of arguments which look like ("_" means a bad value
//	  that we drop):
//
//	   input: A B _ C D _ _ E
//	    Idxs: 0 1 2 3 4 5 6 7
//	 dropped:     2     5 6
//
//	DropKeys(input): A B C D E
//	                 0 1 2 3 4
//
//	OriginalIndex(0) -> 0
//	OriginalIndex(1) -> 1
//	OriginalIndex(2) -> 3
//	OriginalIndex(3) -> 4
//	OriginalIndex(4) -> 7
//
// Methods on this type are NOT goroutine safe.
type DroppedArgTracker []int

// MarkForRemoval tracks `originalIndex` for removal when `Drop*` methods
// are called.
//
// N is a size hint for the maximum number of entries that `dat` could have. If
// `dat` has a capacity of < N, it will be allocated to N.
//
// If called with N == len(args) and originalIndex is always increasing, then
// this will only do one allocation for the life of this DroppedArgTracker, and
// each MarkForRemoval will only cost a single slice append. If called out of
// order, or with a bad value of N, this will do more allocations and will do
// a binary search on each call.
func (dat *DroppedArgTracker) MarkForRemoval(originalIndex, N int) {
	datLen := len(*dat)

	if cap(*dat) < N {
		newDat := make([]int, datLen, N)
		copy(newDat, *dat)
		*dat = newDat
	}

	// most uses will insert linearly, so do a quick check of the max element to
	// see if originalIndex is larger and then do a simple append.
	if datLen == 0 || originalIndex > (*dat)[datLen-1] {
		*dat = append(*dat, originalIndex)
		return
	}

	// Otherwise, search for the correct location and insert it
	insIdx := sort.SearchInts(*dat, originalIndex)
	if insIdx < datLen && (*dat)[insIdx] == originalIndex {
		return
	}
	*dat = append(*dat, 0)
	copy((*dat)[insIdx+1:], (*dat)[insIdx:])
	(*dat)[insIdx] = originalIndex
}

// MarkNilKeys is a helper method which calls MarkForRemoval for each nil key.
func (dat *DroppedArgTracker) MarkNilKeys(keys []*Key) {
	for idx, k := range keys {
		if k == nil {
			dat.MarkForRemoval(idx, len(keys))
		}
	}
}

// MarkNilKeysMeta is a helper method which calls MarkForRemoval for each nil
// key or meta.
func (dat *DroppedArgTracker) MarkNilKeysMeta(keys []*Key, meta MultiMetaGetter) {
	for idx, k := range keys {
		if k == nil || meta[idx] == nil {
			dat.MarkForRemoval(idx, len(keys))
		}
	}
}

// MarkNilKeysVals is a helper method which calls MarkForRemoval for each nil
// key or value.
func (dat *DroppedArgTracker) MarkNilKeysVals(keys []*Key, vals []PropertyMap) {
	for idx, k := range keys {
		if k == nil || vals[idx] == nil {
			dat.MarkForRemoval(idx, len(keys))
		}
	}
}

// If `dat` has a positive length, this will invoke `init` once, followed by
// `include` for every non-overlapping (i, j) range less than N which doesn't
// include any elements indicated with MarkForRemoval.
//
// If `dat` contains a removed index larger than N, this panics.
func (dat DroppedArgTracker) mustCompress(N int, init func(), include func(i, j int)) DroppedArgLookup {
	if len(dat) == 0 || N == 0 {
		return nil
	}

	if largestDropIdx := dat[len(dat)-1]; largestDropIdx >= N {
		panic(errors.Fmt("DroppedArgTracker has out of bound index: %d >= %d ",
			largestDropIdx, N),
		)
	}

	// dal may have len < len(dat) in the event that multiple dat entries are
	// contiguous (they'll be compressed into a single entry in dal).
	dal := make(DroppedArgLookup, 0, len(dat))

	init()
	nextPotentialOriginalInclusion := 0

	for numDroppedSoFar, originalIndexToDrop := range dat {
		if originalIndexToDrop > nextPotentialOriginalInclusion {
			include(nextPotentialOriginalInclusion, originalIndexToDrop)
		}

		nextPotentialOriginalInclusion = originalIndexToDrop + 1
		reducedIndex := originalIndexToDrop - numDroppedSoFar

		if len(dal) != 0 && dal[len(dal)-1].reducedIndex == reducedIndex {
			// If the user drops multiple original indices in a row, we need to
			// reflect them in a single entry in dal.
			dal[len(dal)-1].originalIndex++
		} else {
			// otherwise, we make a new entry.
			dal = append(dal, idxPair{
				reducedIndex,
				nextPotentialOriginalInclusion,
			})
		}
	}
	include(nextPotentialOriginalInclusion, N)

	return dal
}

// DropKeys returns a compressed version of `keys`, dropping all elements which
// were marked with MarkForRemoval.
func (dat DroppedArgTracker) DropKeys(keys []*Key) ([]*Key, DroppedArgLookup) {
	newKeys := keys

	init := func() {
		newKeys = make([]*Key, 0, len(keys)-len(dat))
	}
	include := func(i, j int) {
		newKeys = append(newKeys, keys[i:j]...)
	}

	dal := dat.mustCompress(len(keys), init, include)
	return newKeys, dal
}

// DropKeysAndMeta returns a compressed version of `keys` and `meta`, dropping
// all elements which were marked with MarkForRemoval.
//
// `keys` and `meta` must have the same lengths.
func (dat DroppedArgTracker) DropKeysAndMeta(keys []*Key, meta MultiMetaGetter) ([]*Key, MultiMetaGetter, DroppedArgLookup) {
	newKeys := keys
	newMeta := meta

	// MultiMetaGetter is special and frequently is len 0 with non-nil keys, so we
	// just keep it empty.

	init := func() {
		newKeys = make([]*Key, 0, len(keys)-len(dat))
		if len(meta) > 0 {
			newMeta = make(MultiMetaGetter, 0, len(keys)-len(dat))
		}
	}
	include := func(i, j int) {
		newKeys = append(newKeys, keys[i:j]...)
		if len(meta) > 0 {
			newMeta = append(newMeta, meta[i:j]...)
		}
	}

	dal := dat.mustCompress(len(keys), init, include)
	return newKeys, newMeta, dal
}

// DropKeysAndVals returns a compressed version of `keys` and `vals`, dropping
// all elements which were marked with MarkForRemoval.
//
// `keys` and `vals` must have the same lengths.
func (dat DroppedArgTracker) DropKeysAndVals(keys []*Key, vals []PropertyMap) ([]*Key, []PropertyMap, DroppedArgLookup) {
	newKeys := keys
	newVals := vals

	if len(keys) != len(vals) {
		panic(errors.Fmt("DroppedArgTracker.DropKeysAndVals: mismatched lengths: %d vs %d",
			len(keys), len(vals)),
		)
	}

	init := func() {
		newKeys = make([]*Key, 0, len(keys)-len(dat))
		newVals = make([]PropertyMap, 0, len(keys)-len(dat))
	}
	include := func(i, j int) {
		newKeys = append(newKeys, keys[i:j]...)
		newVals = append(newVals, vals[i:j]...)
	}

	dal := dat.mustCompress(len(keys), init, include)
	return newKeys, newVals, dal
}

type idxPair struct {
	reducedIndex  int
	originalIndex int
}

// DroppedArgLookup is returned from using a DroppedArgTracker.
//
// It can be used to recover the index from the original slice by providing the
// reduced slice index.
type DroppedArgLookup []idxPair

// OriginalIndex maps from an index into the array(s) returned from MustDrop
// back to the corresponding index in the original arrays.
func (dal DroppedArgLookup) OriginalIndex(reducedIndex int) int {
	if len(dal) == 0 {
		return reducedIndex
	}

	// Search for the idxPair whose reducedIndex is LARGER than what we want.
	dalInsertIdx := sort.Search(len(dal), func(dalIdx int) bool {
		return dal[dalIdx].reducedIndex > reducedIndex
	})
	// If search told us that it was "0" it means that no entry in `dal` includes
	// our reducedIndex.
	if dalInsertIdx == 0 {
		return reducedIndex
	}
	// Now look up the idxPair before what search returned. This will have
	// a reducedIndex which is <= reducedIndex.
	entry := dal[dalInsertIdx-1]
	if entry.reducedIndex > reducedIndex {
		return reducedIndex
	}
	// Finally, return the originalIndex and add the difference between our user's
	// reducedIndex and the one in this entry.
	return entry.originalIndex + (reducedIndex - entry.reducedIndex)
}
