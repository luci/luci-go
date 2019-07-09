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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
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
	SendFn func(ctx context.Context, data *buffer.Batch) error

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
	// to Buffer.Retry.
	//
	// Default: Logs the error and returns true on a transient error.
	ErrorFn func(ctx context.Context, failedBatch *buffer.Batch, err error) (retry bool)

	// [OPTIONAL] The maximum number of concurrent invocations of SendFn.
	//
	// Required: Must be > 0
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

	Buffer buffer.Options
}

// Defaults defines the defaults for Options when it contains 0-valued
// fields.
//
// DO NOT ASSIGN/WRITE TO THIS STRUCT.
var Defaults = Options{
	ErrorFn: func(ctx context.Context, failedBatch *buffer.Batch, err error) (retry bool) {
		logging.Errorf(
			ctx,
			"dispatcher.Channel: failed to send Batch(len(Data): %d, Meta: %q): %s",
			len(failedBatch.Data), failedBatch.Meta, err)

		return transient.Tag.In(err)
	},

	MaxSenders: 1,
	MaxQPS:     1.0,
}
