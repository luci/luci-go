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
	// ComputeState evaluates the state of the Buffer (via Stats) and returns:
	//
	//   * okToInsert - User can add item without blocking.
	//   * dropBatch - Buffer needs to drop the oldest batch if the user does
	//     insert data.
	ComputeState(Stats) (okToInsert, dropBatch bool)

	// Check inspects Options to see if it's compatible with this FullBehavior
	// implementation.
	//
	// Called exactly once during Buffer creation.
	Check(Options) error
}

// BlockNewItems prevents the Buffer from accepting any new items as long as it
// it has MaxItems worth of items.
//
// This will never drop batches, but will block inserts.
type BlockNewItems struct {
	// The maximum number of items that this Buffer is allowed to house (including
	// both leased and unleased items).
	//
	// Default: max(1000, BatchSize)
	// Required: Must be >= BatchSize
	MaxItems int
}

var _ FullBehavior = (*BlockNewItems)(nil)

// ComputeState implements FullBehavior.ComputeState.
func (b *BlockNewItems) ComputeState(stats Stats) (okToInsert, dropBatch bool) {
	okToInsert = stats.Total() < b.MaxItems
	return
}

// Check implements FullBehavior.Check.
func (b *BlockNewItems) Check(opts Options) (err error) {
	if b.MaxItems == 0 {
		b.MaxItems = 1000
		if opts.BatchSize > b.MaxItems {
			b.MaxItems = opts.BatchSize
		}
	}
	switch {
	case b.MaxItems >= opts.BatchSize:
	default:
		err = errors.Reason("BlockNewItems.MaxItems must be >= BatchSize[%d]: got %d",
			opts.BatchSize, b.MaxItems).Err()
	}
	return
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
	//     MaxLiveItems + (BatchSize * MaxLeases)
	//
	// Default: max(1000, BatchSize)
	// Required: Must be >= BatchSize
	MaxLiveItems int
}

var _ FullBehavior = (*DropOldestBatch)(nil)

// ComputeState implements FullBehavior.ComputeState.
func (d *DropOldestBatch) ComputeState(stats Stats) (okToInsert, dropBatch bool) {
	okToInsert = true
	if stats.LeasedItemCount+stats.UnleasedItemCount >= d.MaxLiveItems {
		dropBatch = true
	}
	return
}

// Check implements FullBehavior.Check.
func (d *DropOldestBatch) Check(opts Options) (err error) {
	if d.MaxLiveItems == 0 {
		d.MaxLiveItems = 1000
		if opts.BatchSize > d.MaxLiveItems {
			d.MaxLiveItems = opts.BatchSize
		}
	}
	switch {
	case d.MaxLiveItems >= opts.BatchSize:
	default:
		err = errors.Reason("DropOldestBatch.MaxLiveItems must be >= BatchSize[%d]: got %d",
			opts.BatchSize, d.MaxLiveItems).Err()
	}
	return
}

// InfiniteGrowth will not drop data or block new items. It just grows until
// your computer runs out of memory.
//
// This will never block inserts, and will not drop batches.
type InfiniteGrowth struct{}

var _ FullBehavior = InfiniteGrowth{}

// ComputeState implements FullBehavior.ComputeState.
func (i InfiniteGrowth) ComputeState(stats Stats) (okToInsert, dropBatch bool) { return true, false }

// Check implements FullBehavior.Check.
func (i InfiniteGrowth) Check(opts Options) (err error) { return nil }
