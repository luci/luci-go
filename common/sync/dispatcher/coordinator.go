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

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

type coordinatorState struct {
	opts *Options
	buf  *buffer.Buffer

	// The Channel we belong to.
	channel *Channel

	ctx context.Context

	dbg func(fmt string, args ...interface{})

	batchCh  chan *buffer.Batch
	resultCh chan *workerResult

	// Used as a wake-up timer for the coordinator to wake itself up when the
	// buffer will have a batch available due to timeout or reservationToSend is
	// available.
	timer clock.Timer

	// true iff we're supposed to process the rest of or work, but no new work
	// will come in.
	draining bool

	batchToSend       *buffer.Batch
	reservationToSend *rate.Reservation
}

func (c *Channel) mkCoordinatorState(ctx context.Context, opts *Options, buf *buffer.Buffer) coordinatorState {
	return coordinatorState{
		opts: opts,
		buf:  buf,

		batchCh:  make(chan *buffer.Batch),
		resultCh: make(chan *workerResult),

		channel: c,
		ctx:     ctx,
		dbg:     logging.Get(logging.SetField(ctx, "dispatcher.coordinator", true)).Debugf,
		timer:   clock.NewTimer(clock.Tag(ctx, "coordinator")),
	}
}

func (state *coordinatorState) getSendChannel(now time.Time) (send chan<- *buffer.Batch, delay time.Duration) {
	if state.batchToSend != nil {
		// batchToSend and reservationToSend will have been set in a previous loop
		// and are still valid.
		state.dbg("  +resending existing batch")
		send = state.batchCh
		return
	}

	// See if we're permitted to send.
	res := state.opts.QPSLimit.ReserveN(now, 1)
	if !res.OK() {
		panic("impossible")
	}

	if delay = res.DelayFrom(now); delay == 0 {
		state.dbg("  !got reservation")
		// Ok! we're ready to do some work, let's see if we have work to do.
		if state.batchToSend = state.buf.LeaseOne(now); state.batchToSend != nil {
			state.dbg("  !got new batch")
			// We have a reservation and a batch!
			//
			// NOTE: becasue `state.buf` knows about the maximum permitted leases, we
			// can confidently know that if we got a lease here it means that there's
			// a worker goroutine which is ready to take it.
			send = state.batchCh
			state.reservationToSend = res
			return
		}
	}

	state.dbg("  .failed to get batch, canceling reservation")
	res.CancelAt(now)
	return
}

// getNextTimingEvent returns a clock.Timer channel which will activate when the
// earlier of the following happens:
//   * buffer.NextSendTime
//   * resDelay
func (state *coordinatorState) getNextTimingEvent(now time.Time, resDelay time.Duration) (timeEvent <-chan clock.TimerResult) {
	if resDelay == 0 {
		// We already have a batch to send down the pipe, no need to wait for anything.
		return nil
	}

	var resetDuration time.Duration
	var msg string

	if nextSend := state.buf.NextSendTime(); !nextSend.IsZero() && nextSend.After(now) {
		resetDuration = nextSend.Sub(now)
		msg = "waiting on batch.NextSendTime"
	}

	if resDelay < resetDuration {
		resetDuration = resDelay
		msg = "waiting on reservation"
	}

	if resetDuration > 0 {
		state.timer.Reset(resetDuration)
		state.dbg("  |%s (%s)", msg, resetDuration)
		timeEvent = state.timer.GetC()
	}
	return
}

// getWorkChannel returns a channel to recieve an individual work item on (from
// our client) if our buffer is willing to accept additional work items.
//
// Otherwise returns nil.
func (state *coordinatorState) getWorkChannel() (work <-chan interface{}) {
	if !state.draining && state.buf.CanAddItem() {
		state.dbg("  |waiting on new data")
		work = state.channel.itemCh
	}
	return
}

// handleResult is invoked once for each workerResult returned to the
// coordinator from a worker.
//
// This will ACK/NACK the Batch (once).
func (state *coordinatorState) handleResult(result *workerResult) {
	state.dbg("  GOT RESULT")

	if result.err == nil {
		state.dbg("    ACK")
		state.buf.ACK(result.batch)
		return
	}

	state.dbg("    ERR(%s)", result.err)
	if retry := state.opts.ErrorFn(result.batch, result.err); !retry {
		state.dbg("    NO RETRY")
		state.buf.ACK(result.batch)
		return
	}

	state.dbg("    NACK")
	state.buf.NACK(state.ctx, result.err, result.batch)
	return
}

// Returns true iff we're in the draining state and the buffer is now empty.
func (state *coordinatorState) drained() bool {
	return state.draining && state.buf.Stats().Empty()
}

// coordinator is the main goroutine for managing the state of the Channel.
// Exactly one coordinator() function runs per Channel. This coordinates (!!)
// all of the internal channels of the external Channel object in one big select
// loop.
func (c *Channel) coordinator(ctx context.Context, opts *Options, buf *buffer.Buffer, send SendFn) {
	defer close(c.doneCh)

	state := c.mkCoordinatorState(ctx, opts, buf)
	defer close(state.batchCh)
	defer close(state.resultCh)

	defer state.timer.Stop()

	for i := 0; i < state.opts.Buffer.MaxLeases; i++ {
		go state.worker(ctx, i, send)
	}

	for !state.drained() {
		state.dbg("LOOP: buf.Stats[%+v]", state.buf.Stats())

		now := clock.Now(state.ctx)

		// Get this one right away. This populates state.batchToSend, and
		// state.reservationToSend.
		batchCh, resDelay := state.getSendChannel(now)

		select {
		case <-state.ctx.Done():
			break

		case batchCh <- state.batchToSend:
			state.batchToSend = nil
			state.reservationToSend = nil

		case result := <-state.resultCh:
			state.handleResult(result)

		case itm, ok := <-state.getWorkChannel():
			if !ok {
				state.dbg("  GOT DRAIN")
				state.draining = true
				state.buf.Flush(now)
				continue
			}
			state.dbg("  GOT NEW DATA")
			if dropped := state.buf.AddNoBlock(now, itm); dropped != nil {
				state.opts.DropFn(dropped)
				if state.batchToSend != nil && state.batchToSend == dropped {
					state.buf.ACK(dropped)
					state.reservationToSend.CancelAt(now)

					state.batchToSend = nil
					state.reservationToSend = nil
				}
			}

		case result := <-state.getNextTimingEvent(now, resDelay):
			if result.Incomplete() {
				break
			}
			state.dbg("  GOT TIMER WAKEUP")
		}
	}

	state.dbg("DONE")
}
