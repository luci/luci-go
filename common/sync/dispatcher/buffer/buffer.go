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
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
)

// Buffer batches individual data items into Batch objects
type Buffer interface {
	// Adds the item to the buffer.
	//
	// May cut a new Batch according to BatchSize and BatchDuration.
	//
	// If the Buffer is full, applies FullBehavior (panic'ing if FullBehavior ==
	// Block).
	AddNoBlock(context.Context, interface{})

	// Returns the send time for the next-most-available-to-send Batch, or
	// a Zero time.Time if no batches are available to send.
	NextSendTime() time.Time

	// The number of items currently in this Buffer
	Len() int
	// True iff the Buffer will accept an item from AddNoBlock.
	CanAddItem() bool

	// Removes and returns the most-available-to-send Batch from this Buffer.
	//
	// The caller must invoke one of ACK/NACK on the LeasedBatch. The LeasedBatch
	// will count against this Buffer's size until the caller does so.
	//
	// Returns nil if no such Batch exists.
	LeaseOne() *LeasedBatch
}

type bufferImpl struct {
	opts Options

	mu                sync.Mutex
	lastBatchSizes    []int
	lastBatchSizesIdx int
	currentBatch      *Batch

	heap        batchHeap
	itemsInHeap int

	lastBatchID uint64
}

var _ Buffer = (*bufferImpl)(nil)

// NewBuffer returns a new Buffer implementation configured with the given
// Options.
//
// The context will be used with the 'clock' and 'retry' libraries.
func NewBuffer(o Options) Buffer {
	// initialize lastBatchSizes to have a buffer size guess
	last10 := make([]int, 10)
	for i := range last10 {
		last10[i] = o.BatchSizeGuess()
	}

	return &bufferImpl{opts: o, lastBatchSizes: last10}
}

func (buf *bufferImpl) maybePruneLocked(batchID uint64, newBatch bool) bool {
	if buf.Len() < buf.opts.MaxItems {
		return true
	}

	switch buf.opts.FullBehavior {
	case BlockNewItems:
		return !newBatch

	case DropOldestBatch:
		oldestID, oldestIdx := batchID, -1
		for i, batch := range buf.heap {
			if batch.batchID < oldestID {
				oldestID, oldestIdx = batch.batchID, i
			}
		}
		if oldestIdx >= 0 {
			heap.Remove(&buf.heap, oldestIdx)
		}
		return true

	default:
		panic("invalid state")
	}
}

func (buf *bufferImpl) targetBatchSizeLocked() int {
	acc := 0
	for _, value := range buf.lastBatchSizes {
		acc += value
	}
	return (acc / len(buf.lastBatchSizes)) + 1
}

func (buf *bufferImpl) maybeCutBatchLocked(ctx context.Context) {
	batch := buf.currentBatch

	shouldCut := buf.opts.BatchSize > 0 && buf.opts.BatchSize >= len(batch.Data)
	if !shouldCut {
		shouldCut = clock.Now(ctx).After(batch.nextSend)
	}

	if shouldCut {
		batch.countedSize = len(batch.Data)
		heap.Push(&buf.heap, batch)
		buf.itemsInHeap += batch.countedSize
		buf.lastBatchSizes[buf.lastBatchSizesIdx] = batch.countedSize
		buf.lastBatchSizesIdx = (buf.lastBatchSizesIdx + 1) % len(buf.lastBatchSizes)
		buf.currentBatch = nil
	}
}

func (buf *bufferImpl) AddNoBlock(ctx context.Context, item interface{}) {
	buf.mu.Lock()
	defer buf.mu.Unlock()

	if buf.currentBatch == nil {
		buf.currentBatch = &Batch{
			Data:     make([]interface{}, 0, buf.targetBatchSizeLocked()),
			batchID:  buf.lastBatchID + 1,
			retry:    buf.opts.Retry(),
			nextSend: clock.Now(ctx).Add(buf.opts.BatchDuration),
		}
		buf.lastBatchID++
	}

	if canInsert := buf.maybePruneLocked(buf.currentBatch.batchID, true); !canInsert {
		panic("buffer is full and FullBehavior==BlockNewItems")
	}
	buf.currentBatch.Data = append(buf.currentBatch.Data)
	buf.maybeCutBatchLocked(ctx)
}

func (buf *bufferImpl) NextSendTime() time.Time {
	buf.mu.Lock()
	defer buf.mu.Unlock()

	if len(buf.heap) > 0 {
		return buf.heap[0].nextSend
	}

	if buf.currentBatch != nil {
		return buf.currentBatch.nextSend
	}

	return time.Time{}
}

func (buf *bufferImpl) Len() int {
	buf.mu.Lock()
	defer buf.mu.Unlock()

	ret := buf.itemsInHeap
	if buf.currentBatch != nil {
		ret += len(buf.currentBatch.Data)
	}

	return ret
}

func (buf *bufferImpl) CanAddItem() bool {
	switch buf.opts.FullBehavior {
	case DropOldestBatch:
		return true
	case BlockNewItems:
		return buf.Len() < buf.opts.MaxItems
	default:
		panic("invalid state")
	}
}

func (buf *bufferImpl) LeaseOne() *LeasedBatch {
	if len(buf.heap) == 0 {
		return nil
	}
	batch := heap.Pop(&buf.heap).(*Batch)
	return &LeasedBatch{
		batch,
		batch.countedSize,
		buf,
	}
}
