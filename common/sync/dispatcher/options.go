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
	"time"
)

type Options struct {
	// [REQUIRED] This function does the work to actually transmit the Batch to
	// the next stage of your processing pipeline (e.g. do an RPC to a remote
	// service).
	//
	// The function may manipulate the Batch however it wants (see Batch).
	//
	// When this function is invoked, the Batch is removed from the SendBuffer.
	//
	// `lastBatch` will be true if this Channel is closed and there are no more
	// batches to send.
	//
	// The error returned by this function will be handled by ErrorFn.
	SendFn func(*Batch, lastBatch bool) error

	// [OPTIONAL] The minimum amount of time to wait in between invocations of
	// SendFn for a given Batch.
	//
	// Default: 0
	MinSleep time.Duration

	// [OPTIONAL] The minimum amount of time to wait in between invocations of
	// SendFn for a given Batch. The Channel will backoff at
	//
	// Default: 60 * time.Second
	MaxSleep time.Duration

	// [OPTIONAL] The amount to multiply the previous sleep by on every failure
	// of SendFn for a given Batch.
	//
	// Required: If supplied, must be > 1.0
	// Default: 1.2
	BackoffFactor float64

	// [OPTIONAL] The number of concurrent invocations of SendFn.
	//
	// Default: 1
	NumSenders int

	// [OPTIONAL] The maximum number of items to allow in a Batch before adding it
	// to the SendBuffer.
	//
	// Default: 20
	MaxBatchSize int

	// [OPTIONAL] The maximum amount of time to wait before adding a batch to the
	// SendBuffer.
	//
	// This only applies if there are items in the current partialBatch and idle
	// senders.
	//
	// Default: 10 * time.Second
	MaxBatchDuration time.Duration

	// [OPTIONAL] The maximum number of outstanding work items to allow before
	// blocking Channel.WriteChan.
	//
	// If the Channel is currently buffering/sending this many individual items,
	// writing to Channel.WriteChan will block until space is available.
	//
	// Required: Must be >= 0
	// Default: 0  (infinite)
	MaxInFlight int

	// [OPTIONAL] ErrorFn is called to handle the error from SendFn.
	//
	// This may:
	//   * inspect/log the error
	//   * manipulate the contents of failedBatch
	//   * manipulate the contents of buf (e.g. to re-insert the failedBatch)
	//
	// Default: logs the error and re-inserts the Batch if the error is transient.
	ErrorFn   func(buf *SendBuffer, failedBatch *Batch, err error)

	// [OPTIONAL] NewItemFn is invoked whenever a new item is consumed from
	// Channel.WriteChan.
	//
	// This may:
	//   * inspect/log the error
	//   * manipulate the contents of partialBatch (e.g. add itm to it)
	//   * manipulate the contents of buf (e.g. clear it)
	NewItemFn func(buf *SendBuffer, partialBatch *Batch, itm interface{})
}
