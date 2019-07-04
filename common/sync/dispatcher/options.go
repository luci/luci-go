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

package dispatcher

import (
	"context"
	"time"
)

// BufferFullBehavior is an enumeration which tells the dispatcher how to behave
// when its buffer is full (i.e. the number of buffered items is equal to
// MaxBufferedData)
type BufferFullBehavior int

const (
	// BlockNewData will instruct the dispatcher to block all writes to the
	// Channel until the buffer drains at least one datum.
	BlockNewData BufferFullBehavior = 0

	// DropOldestBatch will instruct the dispatcher to drop whichever buffered
	// batch is oldest.
	DropOldestBatch BufferFullBehavior = 1
)

// Options is the configuration options for NewChannel.
type Options struct {
	// [REQUIRED] This function does the work to actually transmit the Batch to
	// the next stage of your processing pipeline (e.g. do an RPC to a remote
	// service).
	//
	// The function may manipulate the Batch however it wants (see Batch).
	//
	// In particular, shrinking the size of Batch.Data for confirmed-sent items
	// will allow the dispatcher to reduce its buffer count when SendFn returns,
	// even if SendFn returns an error. Removing items from the Batch will not
	// cause the remaining items to be coalesced into a different Batch.
	//
	// Non-nil errors returned by this function will be handled by ErrorFn.
	SendFn func(ctx context.Context, data *Batch) error

	// [OPTIONAL] ErrorFn is called to handle the error from SendFn. It executes
	// in the main handler loop of the dispatcher so it can make synchronous
	// decisions about the dispatcher state.
	//
	// Blocking in this function will block ALL dispatcher actions, so be quick
	// :).
	//
	// DO NOT WRITE TO THE CHANNEL DIRECTLY FROM THIS FUNCTION. Doing so will very
	// likely cause deadlocks.
	//
	// This may:
	//   * inspect/log the error
	//   * manipulate the contents of failedBatch
	//   * return a boolean of whether this Batch should be retried or not. If
	//     this is false then the Batch is dropped. If it's true, then it will be
	//     re-queued as-is for transmission according to BufferFullBehavior.
	//   * pass the Batch.Data to another goroutine (in a non-blocking way!) to be
	//     re-queued through Channel.WriteChan.
	//
	// Args:
	//   * ctx - The Context passed to NewChannel
	//   * failedBatch - The Batch for which SendFn produced a non-nil error.
	//   * err - The error SendFn produced.
	//
	// Returns true iff the dispatcher should re-try sending this Batch, according
	// to RetryOptions.
	//
	// Default: Logs the error and returns true on a transient error.
	ErrorFn func(ctx context.Context, failedBatch *Batch, err error) (retry bool)

	Concurrency ConcurrencyOptions

	Retry RetryOptions

	Batch BatchOptions

	Buffer BufferOptions
}

// ConcurrencyOptions configures how the dispatcher does concurrent invocations
// of SendFn.
type ConcurrencyOptions struct {
	// [OPTIONAL] The maximum number of concurrent invocations of SendFn.
	//
	// Default: 1
	MaxSenders int

	// [OPTIONAL] The maximum QPS (SendFn/second) that the dispatcher will allow.
	//
	// After invoking SendFn, the sender thread will wait for an amount of time
	// equal to
	//
	//   max(
	//     0,
	//     ((1s / MaxQPS) / MaxSenders) - duration(SendFn)
	//   )
	//
	// Before accepting the next Batch.
	//
	// Required: Must be > 0
	// Default: 1
	MaxQPS float64
}

// RetryOptions configures how the dispatcher times the retries of failed Batch
// send operations.
type RetryOptions struct {
	// [OPTIONAL] The initial amount of time to wait after a send failure for
	// a given Batch.
	//
	// Required: Must be > 0
	// Default: 200ms
	InitialSleep time.Duration

	// [OPTIONAL] The maximum amount of time to wait in between invocations of
	// SendFn for a given Batch. The Channel will backoff on a per-batch basis
	// using BackoffFactor until it hits MaxSleep.
	//
	// Default: 60 * time.Second
	MaxSleep time.Duration

	// [OPTIONAL] The amount to multiply the previous sleep by on every failure
	// of SendFn for a given Batch.
	//
	// Required: If supplied, must be > 1.0
	// Default: 1.2
	BackoffFactor float64

	// [OPTIONAL] The maximum number of times a single Batch can be tried.
	//
	// Setting this to 1 means that any particular Batch can only have one send
	// attempt.
	//
	// Default: 0 (infinite)
	Limit int
}

// BatchOptions configures how the dispatcher cuts new Batches.
type BatchOptions struct {
	// [OPTIONAL] The maximum number of items to allow in a Batch before queuing
	// it for transmission.
	//
	// Required: Must be >= 0 (If 0, then this relies entirely on MaxDuration)
	// Default: 20
	MaxSize int

	// [OPTIONAL] The maximum amount of time to wait before queuing a Batch for
	// transmission.
	//
	// This only applies if there are buffered items and idle senders.
	//
	// Required: Must be > 0
	// Default: 10 * time.Second
	MaxDuration time.Duration
}

// BufferOptions configures the dispatcher's buffer behavior.
type BufferOptions struct {
	// [OPTIONAL] The maximum number of items to hold in the buffer before
	// enacting BufferFullBehavior.
	//
	// This counts unsent items as well as currently-sending items. When SendFn
	// runs it may also modify the number of items in Batch.Data; Reducing this
	// count will reduce the dispatcher's current buffer size.
	//
	// Default: 1000
	MaxSize int

	// [OPTIONAL] The behavior of the Channel when it currently holds
	// MaxBufferedItems.
	//
	// Default: BlockNewData
	BufferFullBehavior BufferFullBehavior
}
