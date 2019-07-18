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

// Stats is a block of information about the Buffer's present state.
type Stats struct {
	// UnleasedItemCount is the total number of items (i.e. objects passed to
	// AddNoBlock) which are currently owned by the Buffer but are not currently
	// leased. This includes:
	//    * Items not in a ready batch
	//    * Items in unleased batches
	UnleasedItemCount int

	// LeasedItemCount is the total number of items (i.e. objects passed to
	// AddNoBlock) which are currently owned by the Buffer and are in active
	// leases.
	LeasedItemCount int

	// DroppedLeasedItemCount is the total number of items (i.e. objects passed to
	// AddNoBlock) which were part of leases, but where those leases have been
	// dropped.
	DroppedLeasedItemCount int
}

// Empty returns true iff the Buffer is totally empty (has zero user-provided
// items).
func (s Stats) Empty() bool {
	return s.Total() == 0
}

// Total returns the total number of items currently owned by the Buffer.
func (s Stats) Total() int {
	return s.UnleasedItemCount + s.LeasedItemCount + s.DroppedLeasedItemCount
}

// Buffer batches individual data items into Batch objects
//
// All access to the Buffer (as well as invoking ACK/NACK on LeasedBatches) must
// be done from the same goroutine because Buffer is not meant to be
// goroutine-safe.
type Buffer struct {
	opts Options

	batchSizeGuess *movingAverage

	stats Stats

	// the currently-uncut batch; adding data will extend this batch.
	currentBatch *Batch

	// this contains all of the cut, but not yet sent, batches.
	unleased batchHeap

	// this contains all of the live leased batches.
	liveLeases map[*Batch]struct{}

	// this tracks the liveLeases which we've handed out but haven't been ack'd
	// yet.
	knownLeases map[*Batch]struct{}

	lastBatchID uint64
}

// NewBuffer returns a new Buffer implementation configured with the given
// Options.
func NewBuffer(o *Options) (*Buffer, error) {
	if o == nil {
		o = &Options{}
	}
	ret := &Buffer{opts: *o} // copy o before normalizing it

	if err := ret.opts.normalize(); err != nil {
		return nil, errors.Annotate(err, "normalizing buffer.Options").Err()
	}

	ret.batchSizeGuess = newMovingAverage(10, ret.opts.batchSizeGuess())
	ret.liveLeases = map[*Batch]struct{}{}
	ret.knownLeases = map[*Batch]struct{}{}
	return ret, nil
}

// maybeDrop will possibly drop the oldest tracked batch.
//
// Returns ok=true iff it's now OK to insert into the buffer. If this dropped
// a batch, the dropped batch is returned.
func (buf *Buffer) maybeDrop() (dropped *Batch, okToInsert bool) {
	okToInsert, dropBatch := buf.opts.FullBehavior.ComputeState(buf.opts, buf.stats)
	if !dropBatch {
		return
	}

	var pop func() *Batch

	// First search the unleased heap for it's oldest.
	id, idx := buf.unleased.Oldest()
	if idx >= 0 {
		pop = func() *Batch {
			ret := buf.unleased.PopAt(idx)
			buf.stats.UnleasedItemCount -= ret.countedSize
			return ret
		}
	}

	// Now, compare this to the oldest live lease.
	var oldestLease *Batch
	for lb := range buf.liveLeases {
		if id == 0 || lb.id < id {
			oldestLease = lb
			id = lb.id
		}
	}
	if oldestLease != nil {
		pop = func() *Batch {
			delete(buf.liveLeases, oldestLease)
			buf.stats.LeasedItemCount -= oldestLease.countedSize
			buf.stats.DroppedLeasedItemCount += oldestLease.countedSize
			return oldestLease
		}
	}

	if pop == nil {
		// At this point, the only thing left to drop is the currentBatch.
		dropped = buf.currentBatch
		if len(dropped.Data) == 0 {
			panic("impossible")
		}
		buf.currentBatch = nil
		buf.stats.UnleasedItemCount -= len(dropped.Data)
	} else {
		dropped = pop()
	}

	return
}

// AddNoBlock adds the item to the buffer.
//
// May cut a new Batch according to BatchSize.
//
// Possibly drops a batch according to FullBehavior. If
// FullBehavior.ComputeState returns canAdd=false, AddNoBlock will panic.
func (buf *Buffer) AddNoBlock(now time.Time, item interface{}) *Batch {
	dropped, canInsert := buf.maybeDrop()
	if !canInsert {
		panic(errors.New("buffer is full"))
	}

	if buf.currentBatch == nil {
		buf.currentBatch = &Batch{
			// Try to minimize allocations by allocating 20% more slots in Data than
			// the moving average for the last 10 batches' actual use.
			Data:     make([]interface{}, 0, int(buf.batchSizeGuess.get()*1.2)),
			id:       buf.lastBatchID + 1,
			retry:    buf.opts.Retry(),
			nextSend: now.Add(buf.opts.BatchDuration),
		}
		buf.lastBatchID++
	}

	buf.currentBatch.Data = append(buf.currentBatch.Data, item)
	buf.stats.UnleasedItemCount++

	if buf.opts.BatchSize != -1 && len(buf.currentBatch.Data) >= int(buf.opts.BatchSize) {
		buf.Flush(now)
	}
	return dropped
}

// Flush causes any buffered-but-not-batched data to be immediately cut into
// a Batch.
func (buf *Buffer) Flush(now time.Time) {
	batch := buf.currentBatch

	if batch == nil || len(batch.Data) == 0 {
		// len(batch.Data) == 0 could happen if AddNoBlock panic'd due to
		// Buffer fullness and BlockNewItems; a new batch would be allocated, but
		// nothing would be appended to Data.
		return
	}

	batch.countedSize = len(batch.Data)
	batch.nextSend = now // immediately make available to send
	buf.unleased.PushBatch(batch)
	buf.batchSizeGuess.record(batch.countedSize)
	buf.currentBatch = nil
	return
}

// NextSendTime returns the send time for the next-most-available-to-send Batch,
// or a Zero time.Time if no batches are available to send.
func (buf *Buffer) NextSendTime() time.Time {
	ret := time.Time{}

	if len(buf.unleased) > 0 {
		ret = buf.unleased[0].nextSend
	}

	if buf.currentBatch != nil {
		if ns := buf.currentBatch.nextSend; ret.IsZero() || ns.Before(ret) {
			ret = ns
		}
	}

	return ret
}

// Stats returns information about the Buffer's state.
func (buf *Buffer) Stats() Stats {
	return buf.stats
}

// CanAddItem returns true iff the Buffer will accept an item from AddNoBlock.
func (buf *Buffer) CanAddItem() bool {
	canAdd, _ := buf.opts.FullBehavior.ComputeState(buf.opts, buf.stats)
	return canAdd
}

// LeaseOne removes and returns the most-available-to-send Batch from this
// Buffer.
//
// `now` is used to see if we need to cut a new batch due to BatchDuration.
//
// The caller must invoke one of ACK/NACK on the Batch. The Batch
// will count against this Buffer's size until the caller does so.
//
// Returns nil if no batch is available to lease, or if the Buffer has reached
// MaxLeases.
func (buf *Buffer) LeaseOne(now time.Time) (leased *Batch) {
	// Make sure we're even allowed to create another lease before doing anything
	// else.
	if len(buf.knownLeases) >= int(buf.opts.MaxLeases) {
		return
	}

	// if the heap is empty, try to cut a new time-based batch
	if len(buf.unleased) == 0 {
		if cur := buf.currentBatch; cur != nil && len(cur.Data) > 0 {
			// NOTE: !(now < nextSend) == (now >= nextSend)
			if !now.Before(cur.nextSend) {
				buf.Flush(now)
			}
		}
	}

	switch {
	case len(buf.unleased) == 0:
		// still no batches to lease out
	case buf.unleased[0].nextSend.After(now): // nextSend > now
		// batch to lease out is for the future
	default:
		// make the lease
		leased = buf.unleased.PopBatch()
		buf.knownLeases[leased] = struct{}{}
		buf.liveLeases[leased] = struct{}{}
		buf.stats.UnleasedItemCount -= leased.countedSize
		buf.stats.LeasedItemCount += leased.countedSize
	}

	return
}

// ACK records that all the items in the batch have been processed.
//
// The Batch is no longer tracked by the Buffer.
//
// Calling ACK/NACK on the same Batch twice will panic.
// Calling ACK/NACK on a Batch not returned from LeaseOne will panic.
func (buf *Buffer) ACK(leased *Batch) {
	buf.ackImpl(leased)
}

func (buf *Buffer) ackImpl(leased *Batch) (live bool) {
	if _, known := buf.knownLeases[leased]; !known {
		panic(errors.New("unknown *Batch; are you ACK/NACK'ing multiple times?"))
	}
	delete(buf.knownLeases, leased)

	if _, live = buf.liveLeases[leased]; live {
		delete(buf.liveLeases, leased)
		buf.stats.LeasedItemCount -= leased.countedSize
	} else {
		buf.stats.DroppedLeasedItemCount -= leased.countedSize
	}
	return
}

// NACK analyzes the current state of Batch.Data, potentially freeing space in
// the Buffer's heap if the current Buffer.Data length is smaller than when the
// Batch was originally leased.
//
// The Batch will be re-enqueued unless:
//   * The Batch's retry Iterator returns retry.Stop
//   * The Buffer has a DropOldestBatch policy and a new Batch has been added
//     to the Buffer.
//
// Calling ACK/NACK on the same Batch twice will panic.
// Calling ACK/NACK on a Batch not returned from LeaseOne will panic.
func (buf *Buffer) NACK(ctx context.Context, err error, leased *Batch) {
	if live := buf.ackImpl(leased); !live {
		return
	}

	// TODO(iannucci): decouple retry from context (pass in 'now' instead)
	toWait := leased.retry.Next(ctx, err)
	if toWait == retry.Stop {
		return
	}

	leased.nextSend = clock.Now(ctx).Add(toWait)
	if dataSize := len(leased.Data); dataSize < leased.countedSize {
		leased.countedSize = dataSize
	}
	buf.unleased.PushBatch(leased)
	buf.stats.UnleasedItemCount += leased.countedSize

	return
}
