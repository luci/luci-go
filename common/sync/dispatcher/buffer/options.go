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

//go:generate stringer -type FullBehavior

package buffer

import (
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
)

// FullBehavior is an enumeration which tells the dispatcher how to behave
// when its buffer is full (i.e. the number of buffered items is equal to
// MaxBufferedData)
type FullBehavior int

const (
	// BlockNewItems will prevent new items from being added to the Buffer until
	// it's non-full.
	BlockNewItems FullBehavior = iota + 1

	// DropOldestBatch will instruct the dispatcher to drop whichever buffered
	// batch is oldest.
	DropOldestBatch
)

// Options configures how the buffer cuts new Batches.
//
// See Defaults for default values.
type Options struct {
	// [OPTIONAL] The maximum number of items to allow in a Batch before queuing
	// it for transmission.
	//
	// Required: Must be > 0 or -1 (If -1, then this relies entirely on BatchDuration)
	BatchSize int

	// [OPTIONAL] The maximum amount of time to wait before queuing a Batch for
	// transmission.
	//
	// This only applies if there are buffered items and idle senders.
	//
	// Required: Must be > 0
	BatchDuration time.Duration

	// [OPTIONAL] The maximum number of items to hold in the buffer before
	// enacting FullBehavior.
	//
	// This counts unsent items as well as currently-sending items. When SendFn
	// runs it may also modify the number of items in Batch.Data; Reducing this
	// count will reduce the dispatcher's current buffer size.
	//
	// Required: Must be > 0
	MaxItems int

	// [OPTIONAL] The behavior of the Channel when it currently holds
	// MaxItems.
	//
	// Required: Must be a valid FullBehavior.
	FullBehavior FullBehavior

	// [OPTIONAL] Each batch will have a retry.Iterator assigned to it from this
	// retry.Factory.
	//
	// When a Batch is NACK'd, it will be retried at a later time, according the
	// the retry.Iterator.
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
	BatchSize:     20,
	BatchDuration: 10 * time.Second,
	MaxItems:      1000,
	FullBehavior:  BlockNewItems,
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

// Normalize validates that Options is well formed and populates defaults
// which are missing.
func (o *Options) Normalize() error {
	switch {
	case o.BatchSize == 0:
		o.BatchSize = Defaults.BatchSize
	case o.BatchSize > 0 || o.BatchSize == -1:
	default:
		return errors.Reason("BatchSize must be > 0 or == -1: got %d", o.BatchSize).Err()
	}

	if o.BatchDuration == 0 {
		o.BatchDuration = Defaults.BatchDuration
	} else if o.BatchDuration < 0 {
		return errors.Reason("BatchDuration must be > 0: got %s", o.BatchDuration).Err()
	}

	if o.MaxItems == 0 {
		o.MaxItems = Defaults.MaxItems
	} else if o.MaxItems < 0 {
		return errors.Reason("MaxItems must be > 0: got %d", o.MaxItems).Err()
	}

	switch o.FullBehavior {
	case 0:
		o.FullBehavior = Defaults.FullBehavior
	case BlockNewItems, DropOldestBatch:
		// OK
	default:
		return errors.Reason("FullBehavior is unknown: %s", o.FullBehavior).Err()
	}

	if o.Retry == nil {
		o.Retry = Defaults.Retry
	}

	return nil
}
