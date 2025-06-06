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
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
)

// Options configures policy for the Buffer.
//
// See Defaults for default values.
type Options struct {
	// [OPTIONAL] The maximum number of outstanding leases permitted.
	//
	// Attempting additional leases (with LeaseOne) while at the maximum will
	// return nil.
	//
	// Requirement: Must be > 0
	MaxLeases int

	// [OPTIONAL] The maximum number of items to allow in a Batch before making it
	// available to lease.
	//
	// Special value -1: unlimited
	// Requirement: Must be == -1 (i.e. cut batches on BatchAgeMax/BatchSizeMax),
	// or > 0
	BatchItemsMax int

	// [OPTIONAL] The maximum number of "size units" to allow in a Batch before
	// making it available to lease.
	//
	// The units used here are arbitrary and are only checked vs the value
	// provided to AddNoBlock.
	//
	// Size is explicitly provided to AddNoBlock by the caller.
	//
	// Inserting an item which exceeds BatchSizeMax will result in ErrItemTooLarge.
	// It's up to the caller to ensure that this doesn't happen.
	//
	// Special value -1: unlimited
	// Requirement: Must be == -1 (i.e. cut batches on BatchAgeMax/BatchItemsMax),
	// or > 0
	BatchSizeMax int

	// [OPTIONAL] The maximum amount of time to wait before queuing a Batch for
	// transmission. Note that batches are only cut by time when a worker is ready
	// to process them (i.e. LeaseOne is invoked).
	//
	// Requirement: Must be > 0
	BatchAgeMax time.Duration

	// [OPTIONAL] Sets the policy for the Buffer around how many items the Buffer
	// is allowed to hold, and what happens when that number is reached.
	FullBehavior FullBehavior

	// [OPTIONAL] If true, ensures that the next available batch is always the one
	// with the oldest data.
	//
	// If this is false (the default), batches will be leased in the order that
	// they're available to send; If a Batch has a retry with a high delay, it's
	// possible that the next leased Batch actually contains newer data than
	// a later batch.
	//
	// NOTE: if this is combined with high Retry values, it can lead to a
	// head-of-line blocking situation.
	//
	// Requirement: May only be true if MaxLeases == 1
	FIFO bool

	// [OPTIONAL] Each batch will have a retry.Iterator assigned to it from this
	// retry.Factory.
	//
	// When a Batch is NACK'd, it will be retried at "now" plus the Duration
	// returned by the retry.Iterator.
	//
	// If the retry.Iterator returns retry.Stop, the Batch will be silently
	// dropped.
	Retry retry.Factory
}

// Defaults defines the defaults for Options when it contains 0-valued
// fields.
//
// DO NOT ASSIGN/WRITE TO THIS STRUCT.
var Defaults = Options{
	MaxLeases:     4,
	BatchItemsMax: 20,
	BatchSizeMax:  -1,
	BatchAgeMax:   10 * time.Second,
	FullBehavior: &BlockNewItems{
		MaxItems: 1000,
	},
	Retry: func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   200 * time.Millisecond, // initial delay
				Retries: -1,                     // no retry cap
			},
			Multiplier: 1.2,
			MaxDelay:   60 * time.Second,
		}
	},
}

// normalize validates that Options is well formed and populates defaults
// which are missing.
func (o *Options) normalize() error {
	switch {
	case o.MaxLeases == 0:
		o.MaxLeases = Defaults.MaxLeases
	case o.MaxLeases > 0:
	default:
		return errors.Fmt("MaxLeases must be > 0: got %d", o.MaxLeases)
	}

	switch {
	case o.BatchItemsMax == -1:
	case o.BatchItemsMax == 0:
		o.BatchItemsMax = Defaults.BatchItemsMax
	case o.BatchItemsMax > 0:
	default:
		return errors.Fmt("BatchItemsMax must be > 0 or == -1: got %d", o.BatchItemsMax)
	}

	switch {
	case o.BatchSizeMax == -1:
	case o.BatchSizeMax == 0:
		o.BatchSizeMax = Defaults.BatchSizeMax
	case o.BatchSizeMax > 0:
	default:
		return errors.Fmt("BatchSizeMax must be > 0 or == -1: got %d", o.BatchSizeMax)
	}

	switch {
	case o.BatchAgeMax == 0:
		o.BatchAgeMax = Defaults.BatchAgeMax
	case o.BatchAgeMax > 0:
	default:
		return errors.Fmt("BatchAgeMax must be > 0: got %s", o.BatchAgeMax)
	}

	if o.FIFO && o.MaxLeases != 1 {
		return errors.Fmt("FIFO is true, but MaxLeases != 1: got %d", o.MaxLeases)
	}

	if o.FullBehavior == nil {
		o.FullBehavior = Defaults.FullBehavior
	}

	if o.Retry == nil {
		o.Retry = Defaults.Retry
	}

	return errors.WrapIf(o.FullBehavior.Check(*o), "FullBehavior.Check")
}

func (o *Options) batchItemsGuess() int {
	if o.BatchItemsMax > 0 {
		return o.BatchItemsMax
	}
	return 10
}

func (o *Options) checkItemSize(itemSize int) error {
	if itemSize < 0 {
		// We don't ever allow negative sizes.
		return ErrItemTooSmall
	}

	switch {
	case o.BatchSizeMax == -1:
	case itemSize == 0: // reserved for synthetic items
		return ErrItemTooSmall
	case itemSize > o.BatchSizeMax:
		return ErrItemTooLarge
	}
	return nil
}
