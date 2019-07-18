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
	// ComputeState evaluates the state of the Buffer (via Options+Stats) and
	// returns:
	//
	//   * canAdd - User can add item without blocking.
	//   * dropBatch - Does the buffer need to drop the oldest batch to maintain
	//     the constraint.
	ComputeState(Options, Stats) (canAdd, dropBatch bool)

	// Check inspects Options to see if it's compatible with this FullBehavior
	// implementation. It's called exactly once during NewBuffer.
	Check(Options) error
}

// BlockNewItems prevents the Buffer from accepting any new items as long as it
// it has MaxItems worth of items.
//
// This will never drop batches.
type BlockNewItems struct {
	// The maximum number of items that this Buffer is allowed to house (including
	// both leased and unleased items).
	//
	// Default: 1000
	// Required: Must be -1 (infinite) or >= BatchSize
	MaxItems int
}

var _ FullBehavior = (*BlockNewItems)(nil)

func (b *BlockNewItems) ComputeState(opts Options, stats Stats) (canAdd, dropBatch bool) {
	canAdd = b.MaxItems == -1 || stats.Total() < b.MaxItems
	return
}

func (b *BlockNewItems) Check(opts Options) (err error) {
	if b.MaxItems == 0 {
		b.MaxItems = 1000
	}
	switch {
	case b.MaxItems == -1:
	case b.MaxItems >= opts.BatchSize:
	default:
		err = errors.Reason("BlockNewItems.MaxItems must be > BatchSize[%d]: got %d",
			opts.BatchSize, b.MaxItems).Err()
	}
	return
}

// DropOldestBatch will drop buffered data whenever the number of unleased items
// plus leased items would grow beyond MaxLiveItems.
type DropOldestBatch struct {
	// The maximum number of leased and unleased items that the Buffer may have
	// before dropping data.
	//
	// Once a batch is dropped, it no longer counts against MaxLiveItems, but it
	// may still be in memory if the dropped batch was currently leased.
	//
	// NOTE: The Stats.Total number of items the Buffer could have at a given time
	// is:
	//
	//     MaxLiveItems + (BatchSize * MaxLeases)
	//
	// Default: 1000
	// Required: Must be -1 (infinite) or >= BatchSize
	MaxLiveItems int
}

var _ FullBehavior = (*DropOldestBatch)(nil)

func (b *DropOldestBatch) ComputeState(opts Options, stats Stats) (canAdd, dropBatch bool) {
	canAdd = true
	switch {
	case b.MaxLiveItems == -1:
	case stats.LeasedItemCount+stats.UnleasedItemCount >= b.MaxLiveItems:
		dropBatch = true
	}
	return
}

func (b *DropOldestBatch) Check(opts Options) (err error) {
	if b.MaxLiveItems == 0 {
		b.MaxLiveItems = 1000
	}
	switch {
	case b.MaxLiveItems == -1:
	case b.MaxLiveItems >= opts.BatchSize:
	default:
		err = errors.Reason("DropOldestBatch.MaxLiveItems must be > BatchSize[%d]: got %d",
			opts.BatchSize, b.MaxLiveItems).Err()
	}
	return
}
