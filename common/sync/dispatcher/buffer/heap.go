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

// PushBatch pushes a *Batch into the heap.
func (h *batchHeap) PushBatch(batch *Batch) {
	heap.Push(h, batch)
}

// PopBatch pops the lowest *Batch off the heap.
func (h *batchHeap) PopBatch() *Batch {
	return heap.Pop(h).(*Batch)
}

// Oldest finds and returns the oldest batch and its index.
//
// If not found, the returned idx will be -1
func (h *batchHeap) Oldest() (oldest *Batch, idx int) {
	idx = -1
	for i, batch := range *h {
		if oldest == nil || batch.id < oldest.id {
			oldest, idx = batch, i
		}
	}
	return
}

// RemoveAt removes the *Batch at the given index in the heap. Used in
// conjunction with OldestID.
func (h *batchHeap) RemoveAt(idx int) {
	heap.Remove(h, idx)
}
