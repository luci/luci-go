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
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

type coordinatorState[T any] struct {
	opts Options[T]
	buf  *buffer.Buffer[T]

	itemCh  <-chan T
	drainCh chan<- struct{}

	resultCh chan workerResult[T]

	// Used as a wake-up timer for the coordinator to wake itself up when the
	// buffer will have a batch available due to buffer timeout and/or qps limiter.
	timer clock.Timer

	// true if itemCh is closed
	closed bool

	// true if our context is canceled
	canceled bool
}

type workerResult[T any] struct {
	batch *buffer.Batch[T]
	err   error
}

func (state *coordinatorState[T]) dbg(msg string, args ...any) {
	if state.opts.testingDbg != nil {
		state.opts.testingDbg(msg, args...)
	}
}

func (state *coordinatorState[T]) mustSendDueToMinQPS(now, lastSend time.Time) bool {
	mustSend := false
	if !state.closed {
		minInterval := durationFromLimit(state.opts.MinQPS)
		mustSend = minInterval > 0 && now.Sub(lastSend) >= minInterval
	}
	return mustSend
}

// sendBatches sends the batches in buffer.
//
// This can synthesize a new one-item-batch if the item throughput has dipped
// under MinQPS.
//
// It returns the timestamp when the last SendFn was invoked (so, either
// prevLastSend or `now`, if this actually sends a batch), and the delay for how
// long we we need to wait for the next send token due to QPSLimit.
//
// TODO(chanli@): Currently we assume sendBatches is very fast, so we use the same
// now value throughout sendBatches. If it turns out the assumption is false, we
// may have the following issues:
//   - it prevents the QPSLimit from replenishing tokens during sendBatches;
//   - it may cause sendBatches to send an additional synthentic batch
//     immediately after sending out real batches.
func (state *coordinatorState[T]) sendBatches(ctx context.Context, now, prevLastSend time.Time, send SendFn[T]) (lastSend time.Time, delay time.Duration) {
	lastSend = prevLastSend
	if state.canceled {
		for _, batch := range state.buf.ForceLeaseAll() {
			state.dbg("  >dropping batch: canceled")
			state.opts.DropFn(batch, false)
			state.buf.ACK(batch)
		}
		return
	}

	// while the context is not canceled, send all available batches which don't
	// hit one of our limits (QPSLimit, MaxLeases, BatchAgeMax, etc.).
	for ctx.Err() == nil {
		// See if we're permitted to send.
		res := state.opts.QPSLimit.ReserveN(now, 1)
		if !res.OK() {
			panic(errors.New(
				"impossible; Options.QPSLimit is guaranteed to have Inf rate or burst >= 1"))
		}
		if delay = res.DelayFrom(now); delay != 0 {
			// We have to wait until the next send token is available. Cancel the
			// reservation for now, since we're going to wait via getNextTimingEvent.
			res.CancelAt(now)
			return
		}

		// We're allowed to send, see if there's actually anything to send.

		batchToSend := state.buf.LeaseOne(now)

		// We couldn't get a lease, so check if the buffer is empty and we need to
		// satisfy MinQPS.
		if batchToSend == nil && state.buf.Stats().Empty() && state.mustSendDueToMinQPS(now, lastSend) {
			state.dbg("  >synthesizing MinQPSBatch")
			// NOTE: It is critically important that MinQPS be implemented as an
			// actual buffer item (e.g. AddSyntheticNoBlock && LeaseOne).
			//
			// A previous version of this code just sent a `nil` batch instead, but it
			// did this by launching a goroutine to call `send` and push the result to
			// `state.resultCh`. The problem with this is that the main coordinator
			// loop relies entirely on `state.buf.Stats()` to understand the state of
			// in-flight work, and we need ALL invocations of `send` (even synthentic
			// ones like the one below) to:
			//
			//   * respect MaxLeases (otherwise we can end up with more overlapping
			//   SendFn invocations than intended due to MinQPS)
			//   * prevent `run` from returning and closing resultCh until all
			//   goroutines which may push into it have also terminated.
			dropped, err := state.buf.AddSyntheticNoBlock(now)
			if err != nil {
				panic(fmt.Sprintf("impossible - adding minimum-sized item to empty buffer failed: %s", err))
			}
			if dropped != nil {
				panic(fmt.Sprintf("impossible - dropped data from empty buffer: %v", dropped))
			}
			state.buf.Flush(now)
			batchToSend = state.buf.LeaseOne(now)
		}

		if batchToSend != nil {
			state.dbg("  >sending batch")
			lastSend = now
			// NOTE: state.resultCh is closed by run(), which will only close it after
			// the work channel is closed AND state.buf is Empty (meaning no more
			// buffered work, and no leased batches (like the one we got from LeaseOne
			// above).
			//
			// Once this workerResult is pushed into resultCh, the run loop will
			// unblock and consume the result, ACK/NACK'ing the batch to the buffer
			// depending on the state of the channel.
			//
			// ACK/NACK'ing the batch will update the stats of the buffer, which will
			// ultimately allow run() to close and clean up.
			go func() {
				state.resultCh <- workerResult[T]{
					batch: batchToSend,
					err:   send(batchToSend),
				}
			}()
		} else {
			// No more batches, and no MinQPS obligation.
			// Cancel the reservation, since we can't use it.
			res.CancelAt(now)
			break
		}
	}

	return
}

// getNextTimingEvent returns a clock.Timer channel which will activate when the
// later of the following happen:
//   - buffer.NextSendTime or MinQPS, whichever is earlier
//   - nextQPSToken
//
// So resetDuration = max(min(MinQPS, nextSendTime), nextQPSToken)
func (state *coordinatorState[T]) getNextTimingEvent(now time.Time, nextQPSToken time.Duration) <-chan clock.TimerResult {
	var resetDuration time.Duration
	var msg string
	nextSendReached := false

	if nextSend := state.buf.NextSendTime(); !nextSend.IsZero() {
		if nextSend.After(now) {
			resetDuration = nextSend.Sub(now)
			msg = "waiting on batch.NextSendTime"
		} else {
			nextSendReached = true
		}
	}

	minInterval := durationFromLimit(state.opts.MinQPS)
	if !nextSendReached && minInterval > 0 && (resetDuration == 0 || minInterval < resetDuration) {
		resetDuration = minInterval
		msg = "waiting on MinQPS"
	}

	if nextQPSToken > resetDuration {
		resetDuration = nextQPSToken
		msg = "waiting on QPS limit"
	}

	if resetDuration > 0 {
		if !state.timer.Stop() {
			select {
			case <-state.timer.GetC():
			default:
				// The timer was already drained in the main loop.
			}
		}
		state.timer.Reset(resetDuration)
		state.dbg("  |%s (%s)", msg, resetDuration)
		return state.timer.GetC()
	}
	return nil
}

// getWorkChannel returns a channel to receive an individual work item on (from
// our client) if our buffer is willing to accept additional work items.
//
// Otherwise returns nil.
func (state *coordinatorState[T]) getWorkChannel() <-chan T {
	if !state.closed && state.buf.CanAddItem() {
		state.dbg("  |waiting on new data")
		return state.itemCh
	}
	return nil
}

// handleResult is invoked once for each workerResult returned to the
// coordinator from a worker.
//
// This will ACK/NACK the Batch (once).
func (state *coordinatorState[T]) handleResult(ctx context.Context, result workerResult[T]) {
	state.dbg("  GOT RESULT")

	if result.err == nil {
		state.dbg("    ACK")
		state.buf.ACK(result.batch)
		return
	}

	state.dbg("    ERR(%s)", result.err)
	if retry := state.opts.ErrorFn(result.batch, result.err); !retry {
		state.dbg("    NO RETRY (dropping batch)")
		state.opts.DropFn(result.batch, false)
		state.buf.ACK(result.batch)
		return
	}

	if state.canceled {
		state.dbg("    NO RETRY (dropping batch: canceled context)")
		state.opts.DropFn(result.batch, false)
		state.buf.ACK(result.batch)
		return
	}

	state.dbg("    NACK")
	state.buf.NACK(ctx, result.err, result.batch)
}

// coordinator is the main goroutine for managing the state of the Channel.
// Exactly one coordinator() function runs per Channel. This coordinates (!!)
// all of the internal channels of the external Channel object in one big select
// loop.
func (state *coordinatorState[T]) run(ctx context.Context, send SendFn[T]) {
	defer close(state.drainCh)
	if state.opts.DrainedFn != nil {
		defer state.opts.DrainedFn()
	}
	defer state.opts.DropFn(nil, true)
	defer close(state.resultCh)
	defer state.timer.Stop()

	var lastSend time.Time
loop:
	for {
		state.dbg("LOOP (closed: %t, canceled: %t): buf.Stats[%+v]",
			state.closed, state.canceled, state.buf.Stats())

		now := clock.Now(ctx)
		if lastSend.IsZero() {
			// Initiate lastSend to now, otherwise sendBatches will immediately
			// attempt to send a synthentic batch for MinQPS.
			lastSend = now
		}

		var resDelay time.Duration
		lastSend, resDelay = state.sendBatches(ctx, now, lastSend, send)

		// sendBatches may drain the buf if we're in the canceled state, so pull it
		// again to see if it's empty.
		if state.closed && state.buf.Stats().Empty() {
			break loop
		}

		// Only select on ctx.Done if we haven't observed its cancelation yet.
		var doneCh <-chan struct{}
		if !state.canceled {
			doneCh = ctx.Done()
		}

		select {
		case <-doneCh:
			state.dbg("  GOT CANCEL (via context)")
			state.canceled = true
			state.buf.Flush(now)

		case result := <-state.resultCh:
			state.handleResult(ctx, result)

		case itm, ok := <-state.getWorkChannel():
			if !ok {
				state.dbg("  GOT DRAIN")
				state.closed = true
				state.buf.Flush(now)
				continue
			}

			var itemSize int
			if state.opts.ItemSizeFunc != nil {
				itemSize = state.opts.ItemSizeFunc(itm)
			}
			state.dbg("  GOT NEW DATA")
			if state.canceled {
				state.dbg("    dropped item (canceled)")
				state.opts.DropFn(&buffer.Batch[T]{
					Data: []buffer.BatchItem[T]{{Item: itm, Size: itemSize}},
				}, false)
				continue
			}

			dropped, err := state.buf.AddNoBlock(now, itm, itemSize)
			switch err {
			case nil:
			case buffer.ErrItemTooLarge:
				state.dbg("    dropped item (too large)")
			case buffer.ErrItemTooSmall:
				state.dbg("    dropped item (too small)")
			default:
				// "impossible", since the only other possible error is ErrBufferFull,
				// which we should have protected against in getWorkChannel.
				panic(errors.Fmt("unaccounted error from AddNoBlock: %w", err))
			}
			if err != nil {
				state.opts.ErrorFn(&buffer.Batch[T]{
					Data: []buffer.BatchItem[T]{{Item: itm, Size: itemSize}},
				}, err)
				continue
			}
			if dropped != nil {
				state.dbg("    dropped batch")
				state.opts.DropFn(dropped, false)
			}

		case result := <-state.getNextTimingEvent(now, resDelay):
			if result.Incomplete() {
				state.dbg("  GOT CANCEL (via timer)")
				state.canceled = true
				state.buf.Flush(now)
				continue
			}
			state.dbg("  GOT TIMER WAKEUP")
			// opportunistically attempt to send batches; either a new batch is ready
			// to be cut or the qps timer is up. This lowers the upper bound variance
			// and gets a bit closer to the QPS target.
			lastSend, _ = state.sendBatches(ctx, result.Time, lastSend, send)
		}
	}

	state.dbg("DONE")
}
