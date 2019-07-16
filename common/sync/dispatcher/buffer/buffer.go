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

// Package buffer implements a batching buffer with batch lease and retry
// management.
//
// It is meant to be used by something which handles synchronization
// (primarially the "common/sync/dispatcher.Channel"). As such it IS NOT
// GOROUTINE-SAFE.
package buffer

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
)

// Buffer batches individual data items into Batch objects
//
// All access to the Buffer (as well as invoking ACK/NACK on LeasedBatches) must
// be done from the same goroutine because Buffer is not meant to be
// goroutine-safe.
type Buffer interface {
	// Adds the item to the buffer.
	//
	// May cut a new Batch according to BatchSize and BatchDuration.
	//
	// If the Buffer is full, applies FullBehavior:
	//   * BlockNewItems - panics
	//   * DropOldestBatch - returns dropped batch (or nil)
	//
	// Note that this may return a batch which is already leased; This can happen
	// if you add new data to the Buffer which exceeds MaxItems and the
	// currently-leased batch is the oldest.
	AddNoBlock(context.Context, interface{}) *Batch

	// Causes any buffered-but-not-batched data to be cut into a Batch.
	Flush(context.Context)

	// Returns the send time for the next-most-available-to-send Batch, or
	// a Zero time.Time if no batches are available to send.
	NextSendTime() time.Time

	// The number of items currently in this Buffer.
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

// maybePrune will possibly drop a batch from the heap if the heap is too full.
//
// Args:
//   * id is the id of the batch that we're proposing to make space for
//   * newBatch should be true if this is a batch which has never been inserted
//     into the heap before.
//
// Returns ok=true iff it's now OK to insert into the buffer. If this dropped
// a batch, the dropped batch is returned.
func (buf *bufferImpl) maybePrune(id uint64) (dropped *Batch, okToInsert bool) {
	if buf.totalBufferedItems < buf.opts.MaxItems {
		return nil, true
	}

	switch buf.opts.FullBehavior {
	case BlockNewItems:
		return nil, false

	case DropOldestBatch:
		dropped := buf.heap.PopOldestBatchOlderThan(id)
		if dropped != nil {
			buf.totalBufferedItems -= dropped.countedSize
		}
		return dropped, true

	default:
		panic("invalid state")
	}
}

func (buf *bufferImpl) maybeCutBatch(ctx context.Context) {
	batch := buf.currentBatch

	switch {
	case batch == nil:
		return
	case buf.opts.BatchSize > 0 && len(batch.Data) >= buf.opts.BatchSize:
	case clock.Now(ctx).After(batch.nextSend):
	default:
		return
	}

	buf.Flush(ctx)
}

func (buf *bufferImpl) AddNoBlock(ctx context.Context, item interface{}) *Batch {
	buf.maybeCutBatch(ctx) // in case currentBatch is already timed out

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

	dropped, canInsert := buf.maybePrune(buf.currentBatch.id)
	if !canInsert {
		panic(errors.New("buffer is full and FullBehavior==BlockNewItems"))
	}

	buf.currentBatch.Data = append(buf.currentBatch.Data, item)
	buf.totalBufferedItems++

	buf.maybeCutBatch(ctx)
	return dropped
}

func (buf *bufferImpl) Flush(ctx context.Context) {
	batch := buf.currentBatch

	if batch == nil || len(batch.Data) == 0 {
		// len(batch.Data) == 0 could happen if AddNoBlock panic'd due to
		// Buffer fullness and BlockNewItems; a new batch would be allocated, but
		// nothing would be appended to Data.
		return
	}

	batch.countedSize = len(batch.Data)
	batch.nextSend = clock.Now(ctx) // immediately make available to send
	buf.heap.PushBatch(batch)
	buf.batchSizeGuess.record(batch.countedSize)
	buf.currentBatch = nil
	return
}

func (buf *bufferImpl) NextSendTime() time.Time {
	ret := time.Time{}

	if buf.heap.HasUnleasedBatches() {
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
	buf.maybeCutBatch(ctx)
	if !buf.heap.HasUnleasedBatches() {
		return nil
	}

	if clock.Now(ctx).Before(buf.heap[0].nextSend) {
		return nil
	}

	return &LeasedBatch{
		Batch: buf.heap.LeaseBatch(),
		buf:   buf,
	}
}

func (buf *bufferImpl) ackImpl(batch *Batch) {
	if buf.heap.TryDropBatch(batch) {
		buf.totalBufferedItems -= batch.countedSize
	}
	return
}

func (buf *bufferImpl) nackImpl(ctx context.Context, err error, batch *Batch) {
	toWait := batch.retry.Next(ctx, err)
	if toWait == retry.Stop {
		buf.ackImpl(batch)
		return
	}

	leasedSize := batch.countedSize
	if dataSize := len(batch.Data); dataSize < batch.countedSize {
		batch.countedSize = dataSize
	}

	if buf.heap.TryUnleaseBatch(batch) {
		buf.totalBufferedItems -= leasedSize - batch.countedSize
		batch.nextSend = clock.Now(ctx).Add(toWait)
	}
	return
}
