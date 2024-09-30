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

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

// ErrorFn is called to handle the error from SendFn.
//
// This is also invoked with buffer.ErrItemTooLarge if your supplied
// ItemSizeFunc returns a size larger than Buffer.BatchSizeMax (i.e. you pushed
// an item which couldn't fit inside of a Batch). Similarly, if your
// ItemSizeFunc returns <=0, this is invoked with buffer.ErrItemTooSmall.
// Channel ignores the `retry` return value of this function in these cases.
//
// It executes in the main handler loop of the dispatcher so it can make
// synchronous decisions about the dispatcher state.
//
// Blocking in this function will block ALL dispatcher actions, so be quick
// :).
//
// DO NOT WRITE TO THE CHANNEL DIRECTLY FROM THIS FUNCTION. Doing so will very
// likely cause deadlocks.
//
// This may:
//   - inspect/log the error
//   - manipulate the contents of failedBatch
//   - return a boolean of whether this Batch should be retried or not. If
//     this is false then the Batch is dropped. If it's true, then it will be
//     re-queued as-is for transmission according to BufferFullBehavior.
//   - pass the Batch.Data to another goroutine (in a non-blocking way!) to be
//     re-queued through Channel.WriteChan.
//
// Args:
//   - failedBatch - The Batch for which SendFn produced a non-nil error.
//   - err - The error SendFn produced.
//
// Returns true iff the dispatcher should re-try sending this Batch, according
// to Buffer.Retry.
type ErrorFn[T any] func(failedBatch *buffer.Batch[T], err error) (retry bool)

// Options is the configuration options for NewChannel.
type Options[T any] struct {
	// [OPTIONAL] The ErrorFn to use (see ErrorFn docs for details).
	//
	// Default: Logs the error (at Info for retryable errors, and Error for
	// non-retryable errors) and returns true on a transient error.
	ErrorFn ErrorFn[T]

	// [OPTIONAL] Called with the dropped batch any time the Channel drops a batch.
	//
	// This includes:
	//   * When FullBehavior==DropOldestBatch and we get new data.
	//   * When FullBehavior==DropOldestBatch and we attempt to retry old data.
	//   * When ErrorFn returns false for a batch.
	//
	// It executes in the main handler loop of the dispatcher so it can make
	// synchronous decisions about the dispatcher state.
	//
	// Blocking in this function will block ALL dispatcher actions, so be quick
	// :).
	//
	// DO NOT WRITE TO THE CHANNEL DIRECTLY FROM THIS FUNCTION. Doing so will very
	// likely cause deadlocks.
	//
	// When the channel is fully drained, this will be invoked exactly once with
	// `(nil, true)`. This will occur immediately before the DrainedFn is called.
	// Some drop functions buffer their information, and this gives them an
	// opportunity to flush out any buffered data.
	//
	// Default: logs (at Info level if FullBehavior==DropOldestBatch, or Warning
	// level otherwise) the number of data items in the Batch being dropped.
	DropFn func(b *buffer.Batch[T], flush bool)

	// [OPTIONAL] Called exactly once when the associated Channel is closed and
	// has fully drained its buffer, but before DrainC is closed.
	//
	// Note that this takes effect whether the Channel is shut down via Context
	// cancellation or explicitly by closing Channel.C.
	//
	// This is useful for performing final state synchronization tasks/metrics
	// finalization/helpful "everything is done!" messages/etc. without having to
	// poll the Channel to see if it's done and also maintain external
	// synchronization around the finalization action.
	//
	// Called in the main handler loop, but it's called after all other work is
	// done by the Channel, so the only thing it blocks is the closure of DrainC.
	//
	// Default: No action.
	DrainedFn func()

	// [OPTIONAL] A rate limiter for how frequently this will invoke SendFn.
	//
	// Default: No limit.
	QPSLimit *rate.Limiter

	// [OPTIONAL] The minimal frequency of invoking SendFn.
	//
	// If greater than zero, this Channel will invoke SendFn at least this often,
	// as long as:
	//   * the Channel remains open.
	//   * the Channel Buffer has no data (no pending data and no outstanding
	//     SendFn goroutines).
	//
	// If there's a period of time longer than this with no work items where the
	// criteria above matches, Channel will invoke SendFn with a batch containing
	// a single zero-size BatchItem with the `Synthetic` flag set.
	//
	// NOTE: Depending on the Buffer.FullBehavior setting, these synthesized items
	// could cause existing leases to drop (so - if SendFn returns an error, and
	// ErrorFn returns `retry == true`, the channel may end up dropping the batch
	// because some other MinQPSBatch's were synthesized).
	//
	// Errors returned from these SendFn invocations are still processed
	// normally, and synthesized batches still count against QPSLimit.
	//
	// It is an error to specify a MinQPS value which is
	// * greater than QPSLimit.Limit(),
	// * or is rate.Inf.
	//
	// Default: No minimum QPS, no MinQPSBatch batches will be sent.
	MinQPS rate.Limit

	// [OPTIONAL]
	// Should return the size of the given buffer item (i.e. what you push into
	// Channel.C) in whatever units you like (see Buffer.BatchSizeMax).
	//
	// The function will only ever be called once per pushed item.
	//
	// [REQUIRED]
	// Must be non-nil if Buffer.BatchSizeMax is specified.
	//
	// Must return a positive value less than Buffer.BatchSizeMax. Failure to do
	// so will cause `itm` to be immediately rejected from the dispatcher.Channel
	// and routed to ErrorFn with no further processing.
	ItemSizeFunc func(itm T) int

	Buffer buffer.Options

	// Debug output for tests.
	testingDbg func(string, ...any)
}

func defaultDropFnFactory[T any](ctx context.Context, fullBehavior buffer.FullBehavior) func(*buffer.Batch[T], bool) {
	return func(dropped *buffer.Batch[T], flush bool) {
		if flush {
			return
		}
		logFn := logging.Warningf
		if _, ok := fullBehavior.(*buffer.DropOldestBatch); ok {
			logFn = logging.Infof
		}
		logFn(
			ctx,
			"dropping Batch(len(Data): %d, Meta: %+v)",
			len(dropped.Data), dropped.Meta)
	}
}

func defaultErrorFnFactory[T any](ctx context.Context) ErrorFn[T] {
	return func(failedBatch *buffer.Batch[T], err error) (retry bool) {
		retry = transient.Tag.In(err)
		logFn := logging.Errorf
		if retry {
			logFn = logging.Infof
		}
		logFn(
			ctx,
			"failed to send Batch(len(Data): %d, Meta: %+v): %s",
			len(failedBatch.Data), failedBatch.Meta, err)

		return
	}
}

// ErrorFnQuiet is an implementation of Options.ErrorFn which doesn't log the
// batch, but does check for `transient.Tag` to determine `retry`.
func ErrorFnQuiet[T any](b *buffer.Batch[T], err error) (retry bool) {
	return transient.Tag.In(err)
}

// ErrorFnReport is an implementation of Options.ErrorFn which sends all errors
// to a buffered channel. The channel MUST be drained as quickly as possible.
// Otherwise, it may block all dispatcher actions.
//
// If `inner` error function is provided, it is used to determine `retry`.
// Otherwise, `retry` is always false.
func ErrorFnReport[T any](bufferSize int, inner ErrorFn[T]) (ErrorFn[T], <-chan error) {
	errCh := make(chan error, bufferSize)
	return func(b *buffer.Batch[T], err error) bool {
		errCh <- err
		if inner != nil {
			return inner(b, err)
		}
		return false
	}, errCh
}

// DropFnQuiet is an implementation of Options.DropFn which drops batches
// without logging anything.
func DropFnQuiet[T any](*buffer.Batch[T], bool) {}

// DropFnSummarized returns an implementation of Options.DropFn which counts the
// number of dropped batches, and only reports it at the rate provided.
//
// Unlike the default log function, this only logs the number of dropped items
// and the duration that they were collected over.
func DropFnSummarized[T any](ctx context.Context, lim *rate.Limiter) func(*buffer.Batch[T], bool) {
	durationStart := clock.Now(ctx)
	dropCount := 0
	return func(b *buffer.Batch[T], flush bool) {
		dataLen := 0
		if b != nil {
			dataLen = len(b.Data)
		}
		if lim.Allow() || flush {
			now := clock.Now(ctx)
			logging.Infof(
				ctx, "dropped %d items over %s", dropCount+dataLen, now.Sub(durationStart))
			durationStart = now
			dropCount = 0
		} else {
			dropCount += dataLen
		}
	}
}
