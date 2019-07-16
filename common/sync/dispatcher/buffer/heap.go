// Copyright 2019 The LUCI Authors.
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

package buffer

import (
	"container/heap"

	"go.chromium.org/luci/common/errors"
)

// batchHeap maintains sorted order based on (nextSend, id)
type batchHeap []*Batch

var _ heap.Interface = &batchHeap{}

// Implements sort.Interface.
func (h batchHeap) Len() int           { return len(h) }
func (h batchHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h batchHeap) Less(i, j int) bool { return h[i].Less(h[j]) }

// Implements heap.Interface.
func (h *batchHeap) Push(itm interface{}) { *h = append(*h, itm.(*Batch)) }
func (h *batchHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// PushBatch pushes a *Batch into the heap, maintaining the heap invariant.
func (h *batchHeap) PushBatch(batch *Batch) {
	heap.Push(h, batch)
}

// LeaseBatch marks the youngest (-leased?, nextSend, id) *Batch from the heap,
// maintaining the heap invariant.
//
// Panics if there are no unleased batches.
func (h *batchHeap) LeaseBatch() *Batch {
	batch := (*h)[0]
	if batch.currentlyLeased {
		panic(errors.New("cannot lease batch from heap; no unleased batches"))
	}
	batch.currentlyLeased = true
	heap.Fix(h, 0)
	return batch
}

func (h *batchHeap) HasUnleasedBatches() bool {
	return len(*h) > 0 && !(*h)[0].currentlyLeased
}

// TryUnleaseBatch unleases a Batch by pointer. Maintains the heap invariant.
//
// Returns true iff the Batch was unleased. Otherwise the supplied batch was
// dropped at a previous time.
func (h *batchHeap) TryUnleaseBatch(b *Batch) (ok bool) {
	if b.currentlyLeased {
		b.currentlyLeased = false
		heap.Fix(h, h.idxOf(b))
		ok = true
	}
	return
}

// TryDropBatch removes the given batch from the heap. Maintains the heap
// invariant.
//
// Returns true iff the Batch was dropped. Otherwise the supplied batch was
// dropped at a previous time.
func (h *batchHeap) TryDropBatch(b *Batch) (ok bool) {
	if b.currentlyLeased {
		b.currentlyLeased = false
		heap.Remove(h, h.idxOf(b))
		ok = true
	}
	return
}

// PopOldestBatchOlderThan finds, pops and returns the oldest batch whose id is
// less than `id`. If no such batch exists, returns nil without removing
// anything.
func (h *batchHeap) PopOldestBatchOlderThan(id uint64) *Batch {
	oldestID, oldestIdx := id, -1
	for i, batch := range *h {
		if batch.id < oldestID {
			oldestID, oldestIdx = batch.id, i
		}
	}
	if oldestIdx >= 0 {
		ret := heap.Remove(h, oldestIdx).(*Batch)
		ret.currentlyLeased = false
		return ret
	}
	return nil
}

// idxOf returns the index of b in h or panics if b is not in h.
func (h *batchHeap) idxOf(b *Batch) int {
	for i, batch := range *h {
		if batch == b {
			return i
		}
	}
	panic(errors.New("batch does not appear in heap"))
}
