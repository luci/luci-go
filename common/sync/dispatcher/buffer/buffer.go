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
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
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
	// Context is used to see if we need to cut a new batch due to BatchDuration.
	//
	// The caller must invoke one of ACK/NACK on the LeasedBatch. The LeasedBatch
	// will count against this Buffer's size until the caller does so.
	//
	// Returns nil if no such Batch exists.
	LeaseOne(ctx context.Context) *LeasedBatch
}

type bufferImpl struct {
	opts Options

	// This includes:
	//   * Items in currentBatch
	//   * Items in heap
	//   * Items in LeasedBatches which haven'd been ACK/NACK'd
	totalBufferedItems int

	mu sync.Mutex

	batchSizeGuess *movingAverage

	currentBatch *Batch
	heap         batchHeap

	lastBatchID uint64
}

var _ Buffer = (*bufferImpl)(nil)

// NewBuffer returns a new Buffer implementation configured with the given
// Options.
func NewBuffer(o *Options) (Buffer, error) {
	if o == nil {
		o = &Options{}
	}
	ret := &bufferImpl{opts: *o} // copy o before normalizing it

	if err := ret.opts.normalize(); err != nil {
		return nil, errors.Annotate(err, "normalizing buffer.Options").Err()
	}

	ret.batchSizeGuess = newMovingAverage(10, ret.opts.BatchSizeGuess())
	return ret, nil
}

// maybePruneLocked will possibly drop a batch from the heap if the heap is too
// full.
//
// Args:
//   * id is the id of the batch that we're proposing to make space for
//   * newBatch should be true if this is a batch which has never been inserted
//     into the heap before.
//
// Returns true iff it's now OK to insert into the buffer.
func (buf *bufferImpl) maybePruneLocked(id uint64, newBatch bool) bool {
	if buf.totalBufferedItems < buf.opts.MaxItems {
		return true
	}

	switch buf.opts.FullBehavior {
	case BlockNewItems:
		return !newBatch

	case DropOldestBatch:
		dropped := buf.heap.PopOldestBatchOlderThan(id)
		if dropped != nil {
			buf.totalBufferedItems -= dropped.countedSize
		}
		return dropped != nil || newBatch

	default:
		panic("invalid state")
	}
}

func (buf *bufferImpl) maybeCutBatchLocked(ctx context.Context) {
	batch := buf.currentBatch

	switch {
	case batch == nil || len(batch.Data) == 0:
		// len(batch.Data) == 0 could happen if AddNoBlock panic'd due to
		// Buffer fullness and BlockNewItems; a new batch would be allocated, but
		// nothing would be appended to Data.
		return
	case buf.opts.BatchSize > 0 && len(batch.Data) >= buf.opts.BatchSize:
	case clock.Now(ctx).After(batch.nextSend):
	default:
		return
	}

	batch.countedSize = len(batch.Data)
	batch.nextSend = clock.Now(ctx) // immediately make available to send
	buf.heap.PushBatch(batch)
	buf.batchSizeGuess.record(batch.countedSize)
	buf.currentBatch = nil
}

func (buf *bufferImpl) AddNoBlock(ctx context.Context, item interface{}) {
	buf.mu.Lock()
	defer buf.mu.Unlock()

	buf.maybeCutBatchLocked(ctx) // in case currentBatch is already timed out

	if buf.currentBatch == nil {
		buf.currentBatch = &Batch{
			// Try to minimize allocations by allocating 20% more slots in Data than
			// the moving average for the last 10 batches' actual use.
			Data:     make([]interface{}, 0, int(buf.batchSizeGuess.get()*1.2)),
			id:       buf.lastBatchID + 1,
			retry:    buf.opts.Retry(),
			nextSend: clock.Now(ctx).Add(buf.opts.BatchDuration),
		}
		buf.lastBatchID++
	}

	if canInsert := buf.maybePruneLocked(buf.currentBatch.id, true); !canInsert {
		panic(errors.New("buffer is full and FullBehavior==BlockNewItems"))
	}

	buf.currentBatch.Data = append(buf.currentBatch.Data, item)
	buf.totalBufferedItems++
	buf.maybeCutBatchLocked(ctx)
}

func (buf *bufferImpl) NextSendTime() time.Time {
	buf.mu.Lock()
	defer buf.mu.Unlock()

	ret := time.Time{}

	if len(buf.heap) > 0 {
		ret = buf.heap[0].nextSend
	}

	if buf.currentBatch != nil {
		ns := buf.currentBatch.nextSend
		if ret.IsZero() || ns.Before(ret) {
			ret = ns.Add(time.Millisecond)
		}
	}

	return ret
}

func (buf *bufferImpl) Len() int {
	return buf.totalBufferedItems
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

func (buf *bufferImpl) LeaseOne(ctx context.Context) *LeasedBatch {
	buf.mu.Lock()
	defer buf.mu.Unlock()

	buf.maybeCutBatchLocked(ctx)
	if len(buf.heap) == 0 {
		return nil
	}

	if clock.Now(ctx).Before(buf.heap[0].nextSend) {
		return nil
	}

	batch := buf.heap.PopBatch()
	return &LeasedBatch{
		Batch:      batch,
		leasedSize: batch.countedSize,
		buf:        buf,
	}
}

func (buf *bufferImpl) ackImpl(leasedSize int) {
	buf.mu.Lock()
	buf.totalBufferedItems -= leasedSize
	buf.mu.Unlock()
}

func (buf *bufferImpl) nackImpl(ctx context.Context, err error, batch *Batch, leasedSize int) {
	toWait := batch.retry.Next(ctx, err)
	if toWait == retry.Stop {
		buf.ackImpl(leasedSize)
		return
	}
	batch.nextSend = clock.Now(ctx).Add(toWait)

	batch.countedSize = leasedSize
	if dataSize := len(batch.Data); dataSize < batch.countedSize {
		batch.countedSize = dataSize
	}

	buf.mu.Lock()
	heapDiff := 0
	if canInsert := buf.maybePruneLocked(batch.id, false); canInsert {
		heapDiff = batch.countedSize
		buf.heap.PushBatch(batch)
	}
	buf.totalBufferedItems -= leasedSize - heapDiff
	buf.mu.Unlock()
}
