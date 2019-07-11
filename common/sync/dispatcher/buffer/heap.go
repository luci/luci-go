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

	"go.chromium.org/luci/common/data/sortby"
)

// batchHeap maintains sorted order based on (nextSend, id)
type batchHeap []*Batch

var _ heap.Interface = &batchHeap{}

// Len implements sort.Interface.
func (h batchHeap) Len() int { return len(h) }

// Swap implements sort.Interface.
func (h batchHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Less implements sort.Interface.
func (h batchHeap) Less(i, j int) bool {
	return (sortby.Chain{
		func(i, j int) bool { return h[i].nextSend.Before(h[j].nextSend) },
		func(i, j int) bool { return h[i].id < h[j].id },
	}).Use(i, j)
}

// Push adds an element to the end of the underlying list; code in this package
// should use PushBatch instead, which retains the heap invariant.
//
// Implements heap.Interface.
func (h *batchHeap) Push(itm interface{}) {
	*h = append(*h, itm.(*Batch))
}

// Pop removes the last element from the underlying list; code in this package
// should use PopBatch instead, which retains the heap invariant.
//
// Implements heap.Interface.
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

// PopBatch removes the youngest (nextSend, id) *Batch from the heap,
// maintaining the heap invariant.
func (h *batchHeap) PopBatch() *Batch {
	return heap.Pop(h).(*Batch)
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
		return heap.Remove(h, oldestIdx).(*Batch)
	}
	return nil
}
