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

// sendBatches sends the batches in buffer, or a nil batch if the minimum frequency
// has reached.
//
// It returns the timestamp when the last SendFn is invoked, and a delay if we
// need to wait for the next send token.
//
// TODO(chanli@): Currently we assume sendBatches is very fast, so we use the same
// now value throughout sendBatches. If it turns out the assumption is false, we
// may have bellow issues:
// * it prevents the QPSLimit from replenishing tokens during sendBatches;
// * it may causes sendBatches to send an additional nil batch after sending
//
//	batches, while sendBatches should only try to send a nil batch if it doesn't
//	have any batch to send.
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

	// while the context is not canceled, send stuff batches we're able to send.
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
		if batchToSend := state.buf.LeaseOne(now); batchToSend != nil {
			// got a batch! Send it.
			state.dbg("  >sending batch")
			lastSend = now
			go func() {
				state.resultCh <- workerResult[T]{
					batch: batchToSend,
					err:   send(batchToSend),
				}
			}()
		} else {
			// No more batches.

			// If there will be no more batches in the future, break.
			if state.closed {
				res.CancelAt(now)
				break
			}

			// Otherwise, check if the minimal frequency has reached, if yes we
			// need to send a nil batch.
			minInterval := durationFromLimit(state.opts.MinQPS)
			if minInterval > 0 && now.Sub(lastSend) >= minInterval {
				// Send a nil batch.
				state.dbg("  >sending nil batch")
				lastSend = now
				go func() {
					state.resultCh <- workerResult[T]{
						batch: nil,
						err:   send(nil),
					}
				}()
			} else {
				// Cancel the reservation, since we can't use it.
				res.CancelAt(now)
			}
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
	return
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
			// Initiate lastSend to now, otherwise sendBatches will immediately send
			// a nil batch.
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
				panic(errors.Annotate(err, "unaccounted error from AddNoBlock").Err())
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
