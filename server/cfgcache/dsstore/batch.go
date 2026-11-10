// Copyright 2026 The LUCI Authors.
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

package dsstore

import (
	"cmp"
	"slices"
)

type sizer interface {
	size() int64
}

// batch groups items into batches where each batch is ideally targetBatchSize.
//
// Items which are >= 90% of targetBatchSize will be their own batch, and
// batches are allowed to grow up to 110% of targetBatchSize. If
// targetBatchSize <= 0, each item is placed in its own batch.
//
// Reorders `items` in place.
//
// This is used by [Store.Get] to break up multi-point-get operations in a
// reasonable fashion.
//
// This uses a simple algorithm that operates in roughly O(len(items) *
// #batches).
func batch[Item sizer](items []Item, targetBatchSize int64) [][]Item {
	if len(items) == 0 {
		return nil
	}
	if targetBatchSize <= 0 {
		ret := make([][]Item, len(items))
		for i, item := range items {
			ret[i] = []Item{item}
		}
		return ret
	}

	tenth := targetBatchSize / 10
	maxBatchSize90 := targetBatchSize - tenth
	maxBatchSize110 := targetBatchSize + tenth

	slices.SortFunc(items, func(a, b Item) int {
		return cmp.Compare(b.size(), a.size())
	})

	type pendingBatch struct {
		items []Item
		size  int64
	}
	// pending is kept ordered from fullest to emptiest, all with
	// size < targetBatchSize.
	var pending []*pendingBatch
	var ret [][]Item

	addItem := func(item Item) {
		itemSize := max(0, item.size())
		// First of all, if `itemSize` is >= 90% of targetBatchSize, just put it
		// directly in its own batch in ret.
		if itemSize >= maxBatchSize90 {
			ret = append(ret, []Item{item})
			return
		}

		// Try to fit the item into an existing batch (trying largest batches
		// first).
		for i, batch := range pending {
			if itemSize+batch.size <= maxBatchSize110 {
				// Found one; add it in.
				batch.items = append(batch.items, item)
				batch.size += itemSize

				if batch.size >= targetBatchSize {
					// Now the batch is too large, punt it to `ret` and remove this from
					// the slice.
					ret = append(ret, batch.items)
					pending = slices.Delete(pending, i, i+1)
					return
				}

				// Otherwise, bubble the batch up the slice to retain the pending
				// slice order invariant.
				for ; i > 0 && pending[i].size > pending[i-1].size; i-- {
					pending[i], pending[i-1] = pending[i-1], pending[i]
				}
				return
			}
		}

		// None of the pending batches could accept this item, so start a new
		// batch.
		//
		// Because items are *sorted from largest to smallest*, and we fell out of
		// the loop above, itemSize must be less than the smallest existing batch;
		// thus, appending it to the pending batches maintains the sort inviariant.
		pending = append(pending, &pendingBatch{
			items: []Item{item},
			size:  itemSize,
		})
	}

	for _, item := range items {
		addItem(item)
	}

	// Drain the pending batches.
	for _, b := range pending {
		ret = append(ret, b.items)
	}

	return ret
}
