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

// batchHeap maintains sorted order based on
//   - (nextSend, id) if onlyID is false
//   - (id,) if onlyID is true
type batchHeap[T any] struct {
	onlyID bool
	data   []*Batch[T]
}

var _ heap.Interface = &batchHeap[any]{}

// Implements sort.Interface.
func (h batchHeap[T]) Len() int      { return len(h.data) }
func (h batchHeap[T]) Swap(i, j int) { h.data[i], h.data[j] = h.data[j], h.data[i] }
func (h batchHeap[T]) Less(i, j int) bool {
	a, b := h.data[i], h.data[j]

	if !h.onlyID {
		if a.nextSend.Before(b.nextSend) {
			return true
		}
		if b.nextSend.Before(a.nextSend) {
			return false
		}
	}
	return a.id < b.id
}

// Implements heap.Interface.
func (h *batchHeap[T]) Push(itm any) {
	h.data = append(h.data, itm.(*Batch[T]))
}
func (h *batchHeap[T]) Pop() any {
	old := h.data
	n := len(old)
	x := old[n-1]
	h.data = old[:n-1]
	return x
}

// PushBatch pushes a *Batch into the heap.
func (h *batchHeap[T]) PushBatch(batch *Batch[T]) {
	heap.Push(h, batch)
}

// PopBatch pops the lowest *Batch off the heap.
func (h *batchHeap[T]) PopBatch() *Batch[T] {
	return heap.Pop(h).(*Batch[T])
}

// Oldest finds and returns the oldest batch and its index.
//
// If not found, the returned idx will be -1
func (h *batchHeap[T]) Oldest() (oldest *Batch[T], idx int) {
	idx = -1
	for i, batch := range h.data {
		if oldest == nil || batch.id < oldest.id {
			oldest, idx = batch, i
		}
	}
	return
}

// RemoveAt removes the *Batch at the given index in the heap. Used in
// conjunction with OldestID.
func (h *batchHeap[T]) RemoveAt(idx int) {
	heap.Remove(h, idx)
}

// Peek returns the lowest-ordered Batch, or nil if the heap is empty.
func (h batchHeap[T]) Peek() *Batch[T] {
	if len(h.data) == 0 {
		return nil
	}
	return h.data[0]
}
