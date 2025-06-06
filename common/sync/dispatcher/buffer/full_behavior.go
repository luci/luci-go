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
	"go.chromium.org/luci/common/errors"
)

// FullBehavior allows you to customize the Buffer's behavior when it gets too
// full.
//
// Generally you'll pick one of DropOldestBatch or BlockNewItems.
type FullBehavior interface {
	// Check inspects Options to see if it's compatible with this FullBehavior
	// implementation.
	//
	// Called exactly once during Buffer creation before any other methods are
	// used.
	Check(Options) error

	// ComputeState evaluates the state of the Buffer (via Stats) and returns:
	//
	//   * okToInsert - User can add an item without blocking.
	//   * dropBatch - Buffer needs to drop the oldest batch if the user does
	//     insert data.
	//
	// Called after Check.
	ComputeState(stats Stats) (okToInsert, dropBatch bool)

	// NeedsItemSize should return true iff this FullBehavior requires item sizes
	// to effectively apply its policy.
	//
	// Called after Check.
	NeedsItemSize() bool
}

// BlockNewItems prevents the Buffer from accepting any new items as long as it
// it has MaxItems worth of items.
//
// This will never drop batches, but will block inserts.
type BlockNewItems struct {
	// The maximum number of items that this Buffer is allowed to house (including
	// both leased and unleased items).
	//
	// Default: -1 if BatchItemsMax != -1 else max(1000, BatchItemsMax)
	// Required: Must be >= BatchItemsMax or -1 (only MaxSize applies)
	MaxItems int

	// The maximum* number of 'size units' that this Buffer is allowed to house
	// (including both leased and unleased items).
	//
	// NOTE(*): This only blocks addition of new items once the buffer is
	// at-or-past capacitiy. e.g. if the buffer has a MaxSize of 1000 and
	// a BatchSizeMax of 100, and you insert items worth 999 units, this will
	// still allow the addition of another item (of up to 100 units) before
	// claiming the buffer is full.
	//
	// NOTE: This may only be set if BatchSizeMax is > 0; Otherwise buffer will
	// not enforce item sizes, and so BlockNewItems will not be able to enforce
	// this policy.
	//
	// Default: -1 if BatchSizeMax != -1 else BatchSizeMax * 5
	// Required: Must be >= BatchSizeMax or -1 (only MaxItems applies)
	MaxSize int
}

var _ FullBehavior = (*BlockNewItems)(nil)

// ComputeState implements FullBehavior.ComputeState.
func (b *BlockNewItems) ComputeState(stats Stats) (okToInsert, dropBatch bool) {
	itemsOK := b.MaxItems == -1 || stats.Total() < b.MaxItems
	sizeOK := b.MaxSize == -1 || stats.TotalSize() < b.MaxSize
	okToInsert = itemsOK && sizeOK
	return
}

// Check implements FullBehavior.Check.
func (b *BlockNewItems) Check(opts Options) (err error) {
	if b.MaxItems == 0 {
		b.MaxItems = negOneOrMax(opts.BatchItemsMax, 1000)
	}
	if b.MaxSize == 0 {
		b.MaxSize = negOneOrMult(opts.BatchSizeMax, 5)
	}

	switch {
	case b.MaxItems == -1:
	case b.MaxItems < opts.BatchItemsMax:
		return errors.Fmt("BlockNewItems.MaxItems must be >= BatchItemsMax[%d]: got %d",
			opts.BatchItemsMax, b.MaxItems)
	}

	switch {
	case b.MaxSize == -1:
	case b.MaxSize < opts.BatchSizeMax:
		return errors.Fmt("BlockNewItems.MaxSize must be >= BatchSizeMax[%d]: got %d",
			opts.BatchSizeMax, b.MaxSize)
	case opts.BatchSizeMax == -1:
		return errors.Fmt("BlockNewItems.MaxSize may only be set with BatchSizeMax[%d] > 0",
			opts.BatchSizeMax)
	}

	if b.MaxItems == -1 && b.MaxSize == -1 {
		return errors.New("BlockNewItems must have one of MaxItems or MaxSize > 0")
	}

	return
}

// NeedsItemSize implements FullBehavior.NeedsItemSize.
func (b *BlockNewItems) NeedsItemSize() bool {
	return b.MaxSize > 0
}

// DropOldestBatch will drop buffered data whenever the number of unleased items
// plus leased items would grow beyond MaxLiveItems.
//
// This will never block inserts, but will drop batches.
type DropOldestBatch struct {
	// The maximum number of leased and unleased items that the Buffer may have
	// before dropping data.
	//
	// Once a batch is dropped, it no longer counts against MaxLiveItems, but it
	// may still be in memory if the dropped batch was currently leased.
	//
	// NOTE: The maximum Stats.Total number of items the Buffer could have at
	// a given time is:
	//
	//     MaxLiveItems + (BatchItemsMax * MaxLeases)
	//
	// Default: -1 if BatchItemsMax == -1 else max(1000, BatchItemsMax)
	// Required: Must be >= BatchItemsMax or -1 (only MaxLiveSize applies)
	MaxLiveItems int

	// The maximum number of leased and unleased size units that the Buffer may
	// have before dropping data.
	//
	// Once a batch is dropped, it no longer counts against MaxLiveSize, but it
	// may still be in memory if the dropped batch was currently leased at the
	// time it was dropped.
	//
	// NOTE: The maximum Stats.TotalSize the Buffer could have at a given
	// time is:
	//
	//     MaxLiveSize + (BatchSizeMax * MaxLeases)
	//
	// NOTE: This may only be set if BatchSizeMax is > 0; Otherwise buffer will
	// not size inserted items, and so DropOldestBatch will not be able to enforce
	// this policy.
	//
	// Default: -1 if BatchSizeMax == -1 else BatchSizeMax * 5
	// Required: Must be >= BatchSizeMax or -1 (only MaxLiveItems applies)
	MaxLiveSize int
}

var _ FullBehavior = (*DropOldestBatch)(nil)

// ComputeState implements FullBehavior.ComputeState.
func (d *DropOldestBatch) ComputeState(stats Stats) (okToInsert, dropBatch bool) {
	okToInsert = true
	itemsDrop := d.MaxLiveItems > -1 && stats.Total() >= d.MaxLiveItems
	sizeDrop := d.MaxLiveSize > -1 && stats.TotalSize() >= d.MaxLiveSize
	dropBatch = itemsDrop || sizeDrop
	return
}

// Check implements FullBehavior.Check.
func (d *DropOldestBatch) Check(opts Options) (err error) {
	if d.MaxLiveItems == 0 {
		d.MaxLiveItems = negOneOrMax(opts.BatchItemsMax, 1000)
	}
	if d.MaxLiveSize == 0 {
		d.MaxLiveSize = negOneOrMult(opts.BatchSizeMax, 5)
	}

	switch {
	case d.MaxLiveItems == -1:
	case d.MaxLiveItems < opts.BatchItemsMax:
		return errors.Fmt("DropOldestBatch.MaxLiveItems must be >= BatchItemsMax[%d]: got %d",
			opts.BatchItemsMax, d.MaxLiveItems)
	}

	switch {
	case d.MaxLiveSize == -1:
	case d.MaxLiveSize < opts.BatchSizeMax:
		return errors.Fmt("DropOldestBatch.MaxLiveSize must be >= BatchSizeMax[%d]: got %d",
			opts.BatchSizeMax, d.MaxLiveSize)
	case opts.BatchSizeMax == -1:
		return errors.Fmt("DropOldestBatch.MaxLiveSize may only be set with BatchSizeMax[%d] > 0",
			opts.BatchSizeMax)
	}

	if d.MaxLiveItems == -1 && d.MaxLiveSize == -1 {
		return errors.New("DropOldestBatch must have one of MaxLiveItems or MaxLiveSize > 0")
	}

	return
}

// NeedsItemSize implements FullBehavior.NeedsItemSize.
func (d *DropOldestBatch) NeedsItemSize() bool {
	return d.MaxLiveSize > 0
}

// InfiniteGrowth will not drop data or block new items. It just grows until
// your computer runs out of memory.
//
// This will never block inserts, and will not drop batches.
type InfiniteGrowth struct{}

var _ FullBehavior = InfiniteGrowth{}

// ComputeState implements FullBehavior.ComputeState.
func (i InfiniteGrowth) ComputeState(Stats) (okToInsert, dropBatch bool) { return true, false }

// Check implements FullBehavior.Check.
func (i InfiniteGrowth) Check(opts Options) (err error) { return nil }

// NeedsItemSize implements FullBehavior.NeedsItemSize.
func (i InfiniteGrowth) NeedsItemSize() bool { return false }

func negOneOrMult(a, b int) int {
	if a == -1 {
		return -1
	}
	return a * b
}

func negOneOrMax(a, b int) int {
	if a == -1 {
		return -1
	}
	return max(a, b)
}
