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

// batchHeap maintains sorted order based on (nextSend, batchID)
type batchHeap []*Batch

var _ heap.Interface = &batchHeap{}

func (h batchHeap) Len() int      { return len(h) }
func (h batchHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h batchHeap) Less(i, j int) bool {
	a, b := h[i], h[j]
	return (sortby.Chain{
		func(_, _ int) bool { return a.nextSend.Before(b.nextSend) },
		func(_, _ int) bool { return a.batchID < b.batchID },
	}).Use(i, j)
}

func (h *batchHeap) Push(itm interface{}) {
	*h = append(*h, itm.(*Batch))
}

func (h *batchHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
