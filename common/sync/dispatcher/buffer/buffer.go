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
// (primarially the "common/sync/dispatcher.Channel"). As such, it IS NOT
// GOROUTINE-SAFE. If you use this outside of dispatcher.Channel, you must
// synchronize all access to the methods of Buffer.
package buffer

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
)

// These errors can be returned from AddNoBlock.
var (
	ErrBufferFull   = errors.New("buffer is full")
	ErrItemTooLarge = errors.New("item exceeds buffer's BatchSizeMax")
	ErrItemTooSmall = errors.New("item has zero or negative size, and BatchSizeMax is set")
)

// Buffer batches individual data items into Batch objects.
//
// All access to the Buffer (as well as invoking ACK/NACK on LeasedBatches) must
// be synchronized because Buffer is not goroutine-safe.
type Buffer struct {
	// User-provided Options block; dictates all policy for this Buffer.
	opts Options

	// A moving average of the number of batch items so far; Helps reduce the
	// number of spurious re-allocations if the average number of batch items is
	// roughly (within 20%) of a constant amount.
	batchItemsGuess *movingAverage

	// Current statistics for this Buffer. Can be read with the Stats() method,
	// and is kept up to date by various functions in the Buffer.
	stats Stats

	// Currently-accumulating batch; Data added to the Buffer will extend this
	// batch.
	//
	// NOTE: It is possible for this to be nil; if AddNoBlock fills this Batch up
	// until Batch.canAccept returns false, it will be removed and pushed into
	// `unleased`.
	currentBatch *Batch

	// Contains all of the cut, but not currently leased, Batches.
	//
	// Kept ordered by (nextSend, id), so the 0th element is always the
	// next-batch-to-be-sent.
	unleased batchHeap

	// Contains all of the live leased batches.
	liveLeases map[*Batch]struct{}

	// Tracks the liveLeases which haven't been ack'd yet.
	//
	// Batches can be removed from liveLeases without being removed from
	// unAckedLeases when the Batch is dropped from the Buffer due to the
	// FullBehavior policy.
	unAckedLeases map[*Batch]struct{}

	// The `id` of the last Batch we cut. If currentBatch is non-nil, this is
	// `currentBatch.id`.
	//
	// NOTE: 0 is not a valid Batch.id.
	lastBatchID uint64
}

// NewBuffer returns a new Buffer configured with the given Options.
//
// If there's an issue with the provided Options, this returns an error.
func NewBuffer(o *Options) (*Buffer, error) {
	if o == nil {
		o = &Options{}
	}
	ret := &Buffer{opts: *o} // copy o before normalizing it

	if err := ret.opts.normalize(); err != nil {
		return nil, errors.Annotate(err, "normalizing buffer.Options").Err()
	}

	ret.unleased.onlyID = o.FIFO
	ret.batchItemsGuess = newMovingAverage(10, ret.opts.batchItemsGuess())
	ret.liveLeases = map[*Batch]struct{}{}
	ret.unAckedLeases = map[*Batch]struct{}{}
	return ret, nil
}

// Returns the oldest *Batch from liveLeases.
func (buf *Buffer) oldestLiveLease() (oldest *Batch) {
	for lb := range buf.liveLeases {
		if oldest == nil || lb.id < oldest.id {
			oldest = lb
		}
	}
	return
}

// dropOldest will drop the oldest un-dropped batch.
//
// Returns the dropped Batch.
func (buf *Buffer) dropOldest() (dropped *Batch) {
	unleased, unleasedIdx := buf.unleased.Oldest()
	leased := buf.oldestLiveLease()

	switch {
	case unleased != nil && (leased == nil || unleased.id < leased.id):
		buf.unleased.RemoveAt(unleasedIdx)
		buf.stats.del(unleased, categoryUnleased)
		return unleased

	case leased != nil:
		delete(buf.liveLeases, leased)
		buf.stats.mv(leased, categoryLeased, categoryDropped)
		return leased

	default:
		// At this point, the only thing left to drop is the currentBatch.
		current := buf.currentBatch
		if current == nil {
			panic(errors.New(
				"impossible; must drop Batch, but there's NO undropped data"))
		}
		buf.currentBatch = nil
		buf.stats.del(current, categoryUnleased)
		return current
	}
}

// AddNoBlock adds the item to the Buffer with the given size.
//
// Possibly drops a batch according to FullBehavior.
//
// Returns an error under the following conditions:
//
//  - ErrBufferFull - If FullBehavior.ComputeState returns okToInsert=false.
//  - ErrItemTooLarge - If this buffer has a BatchSizeMax configured and
//    `itemSize` is larger than this.
//  - ErrItemTooSmall - If this buffer has a BatchSizeMax configured and
//    `itemSize` is zero, or if `itemSize` is negative.
func (buf *Buffer) AddNoBlock(now time.Time, item interface{}, itemSize int) (dropped *Batch, err error) {
	if err = buf.opts.checkItemSize(itemSize); err != nil {
		return
	}
	okToInsert, dropBatch := buf.opts.FullBehavior.ComputeState(buf.stats)
	if !okToInsert {
		return nil, ErrBufferFull
	}
	// All error checks done above before we modify any state in buf.

	if dropBatch {
		dropped = buf.dropOldest()
	}

	if !buf.currentBatch.canAccept(&buf.opts, itemSize) {
		buf.Flush(now)
	}

	if buf.currentBatch == nil {
		buf.currentBatch = &Batch{
			// Try to minimize allocations by allocating 20% more slots in Data than
			// the moving average for the last 10 batches' actual use.
			Data:     make([]BatchItem, 0, int(buf.batchItemsGuess.get()*1.2)),
			id:       buf.lastBatchID + 1,
			retry:    buf.opts.Retry(),
			nextSend: now.Add(buf.opts.BatchAgeMax),
		}
		buf.lastBatchID++
	}

	buf.currentBatch.Data = append(buf.currentBatch.Data, BatchItem{item, itemSize})
	buf.currentBatch.countedItems++
	buf.currentBatch.countedSize += itemSize
	buf.stats.addOneUnleased(itemSize)

	// If the currentBatch couldn't even accept the smallest size item, go ahead
	// and Flush it now.
	if !buf.currentBatch.canAccept(&buf.opts, 1) {
		buf.Flush(now)
	}
	return
}

// Flush causes any buffered-but-not-batched data to be immediately cut into
// a Batch.
//
// No-op if there's no such data.
func (buf *Buffer) Flush(now time.Time) {
	batch := buf.currentBatch
	if batch == nil {
		return
	}

	batch.nextSend = now // immediately make available to send
	buf.unleased.PushBatch(batch)
	buf.batchItemsGuess.record(batch.countedItems)
	buf.currentBatch = nil
	return
}

// NextSendTime returns the send time for the next-most-available-to-send Batch,
// or a Zero time.Time if no batches are available to send.
//
// NOTE: Because LeaseOne enforces MaxLeases, this time may not guarantee an
// available lease.
func (buf *Buffer) NextSendTime() time.Time {
	ret := time.Time{}

	if next := buf.unleased.Peek(); next != nil {
		ret = next.nextSend
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

// CanAddItem returns true iff the Buffer will accept an item from AddNoBlock
// without returning ErrBufferFull.
func (buf *Buffer) CanAddItem() bool {
	okToInsert, _ := buf.opts.FullBehavior.ComputeState(buf.stats)
	return okToInsert
}

// LeaseOne returns the most-available-to-send Batch from this Buffer.
//
// The caller must invoke one of ACK/NACK on the Batch. The Batch will count
// against this Buffer's Stats().Total() until the caller does so.
//
// Returns nil if no batch is available to lease, or if the Buffer has reached
// MaxLeases.
func (buf *Buffer) LeaseOne(now time.Time) (leased *Batch) {
	cur, next := buf.currentBatch, buf.unleased.Peek()

	switch {
	case len(buf.unAckedLeases) == int(buf.opts.MaxLeases):
		// too many outstanding leases
		return

	case next != nil && !now.Before(next.nextSend):
		// next unleased batch is fine to use

	case cur != nil && !now.Before(cur.nextSend):
		// currentBatch has data we can send
		buf.Flush(now)

	default:
		// Nothing's ready.
		return
	}

	return buf.forceLeaseOne()
}

// leases the next available batch, regardless of its marked nextSend time.
func (buf *Buffer) forceLeaseOne() (leased *Batch) {
	if buf.unleased.Len() == 0 {
		return nil
	}

	leased = buf.unleased.PopBatch()
	buf.unAckedLeases[leased] = struct{}{}
	buf.liveLeases[leased] = struct{}{}
	buf.stats.mv(leased, categoryUnleased, categoryLeased)
	return
}

// ForceLeaseAll leases and returns all unleased Batches immediately, regardless
// of their send times.
//
// This is useful for cancelation scenarios where you no longer want to do
// full processing on the remaining batches.
//
// NOTE: It's helpful to call Flush before ForceLeaseAll to include the
// currently buffered, but unbatched, data.
func (buf *Buffer) ForceLeaseAll() []*Batch {
	if buf.unleased.Len() == 0 {
		return nil
	}
	ret := make([]*Batch, 0, buf.unleased.Len())
	for buf.unleased.Len() > 0 {
		ret = append(ret, buf.forceLeaseOne())
	}
	return ret
}

// ACK records that all the items in the batch have been processed.
//
// The Batch is no longer tracked in any form by the Buffer.
//
// Calling ACK/NACK on the same Batch twice will panic.
// Calling ACK/NACK on a Batch not returned from LeaseOne will panic.
func (buf *Buffer) ACK(leased *Batch) {
	buf.removeLease(leased)
}

func (buf *Buffer) removeLease(leased *Batch) (live bool) {
	if leased == nil {
		// Nil batch. Bail out.
		return
	}

	if _, known := buf.unAckedLeases[leased]; !known {
		panic(errors.New("unknown *Batch; are you ACK/NACK'ing multiple times?"))
	}
	delete(buf.unAckedLeases, leased)

	if _, live = buf.liveLeases[leased]; live {
		delete(buf.liveLeases, leased)
		buf.stats.del(leased, categoryLeased)
	} else {
		buf.stats.del(leased, categoryDropped)
	}
	return
}

// NACK analyzes the current state of Batch.Data, potentially reducing
// UnleasedItemCount in the Buffer's if the given Batch.Data length is
// smaller than when the Batch was originally leased.
//
// The Batch will be re-enqueued unless:
//   * The Batch's retry Iterator returns retry.Stop
//   * The Batch has been dropped already due to FullBehavior policy. If this
//     is the case, AddNoBlock would already have returned this *Batch pointer
//     to you.
//
// Calling ACK/NACK on the same Batch twice will panic.
// Calling ACK/NACK on a Batch not returned from LeaseOne will panic.
func (buf *Buffer) NACK(ctx context.Context, err error, leased *Batch) {
	if live := buf.removeLease(leased); !live {
		return
	}

	// TODO(iannucci): decouple retry from context (pass in 'now' instead)
	switch toWait := leased.retry.Next(ctx, err); {
	case toWait == retry.Stop:
		return
	default:
		leased.nextSend = clock.Now(ctx).Add(toWait)
	}

	leased.countedItems = intMin(len(leased.Data), leased.countedItems)
	var newSize int
	for i := range leased.Data {
		newSize += leased.Data[i].Size
	}
	leased.countedSize = intMin(newSize, leased.countedSize)

	buf.unleased.PushBatch(leased)
	buf.stats.add(leased, categoryUnleased)

	return
}

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
