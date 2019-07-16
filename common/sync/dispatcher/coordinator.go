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
	buf  buffer.Buffer

	// The Channel we belong to.
	channel *Channel

	ctx context.Context

	dbg func(fmt string, args ...interface{})

	batchCh  chan *buffer.LeasedBatch
	resultCh chan *workerResult

	// Used as a wake-up timer for the coordinator to wake itself up when the
	// buffer will have a batch available due to timeout or reservationToSend is
	// available.
	timer clock.Timer

	// true iff we're supposed to process the rest of or work, but no new work
	// will come in.
	draining bool

	// The batch we'll send to the workers next. Because the coordinator loop only
	// does one action per loop, this may persist for multiple loops. If this is
	// nil, may be populated by getBatchSendChannel.
	batchToSend *buffer.LeasedBatch

	// This is a reservation from the QPS Limiter. We need this and batchToSend to
	// both be populated to actually send work. This is reset when the batch is
	// actually sent to a worker.
	reservationToSend *rate.Reservation
}

func (c *Channel) mkCoordinatorState(ctx context.Context, opts *Options, buf buffer.Buffer) coordinatorState {
	return coordinatorState{
		opts: opts,
		buf:  buf,

		batchCh:  make(chan *buffer.LeasedBatch),
		resultCh: make(chan *workerResult),

		channel: c,
		ctx:     ctx,
		dbg:     logging.Get(logging.SetField(ctx, "dispatcher.coordinator", true)).Debugf,
		timer:   clock.NewTimer(clock.Tag(ctx, "coordinator")),
	}
}

// updateSendState updates `batchToSend` and `reservationToSend` if either is
// nil.
func (state *coordinatorState) updateSendState(now time.Time) {
	if state.batchToSend == nil {
		state.batchToSend = state.buf.LeaseOne(state.ctx)
		if state.batchToSend != nil {
			state.dbg("  >new batch")
		}
	}
	if state.reservationToSend == nil {
		res := state.opts.QPSLimit.ReserveN(now, 1)
		if !res.OK() {
			panic("impossible; QPSLimit.Burst() < 1")
		}
		state.dbg("  >new reservation")
		state.reservationToSend = res
	}
}

func (state *coordinatorState) getSendChannel(now time.Time) (send chan<- *buffer.LeasedBatch) {
	if state.batchToSend != nil && state.reservationToSend.DelayFrom(now) <= 0 {
		state.dbg("  |waiting to send batch")
		send = state.batchCh
	}
	return
}

// getNextTimingEvent returns a clock.Timer channel which will activate when the
// earlier of the following happens:
//   * buffer.NextSendTime
//   * reservationToSend.Delay
func (state *coordinatorState) getNextTimingEvent(now time.Time) (timeEvent <-chan clock.TimerResult) {
	var resetDuration time.Duration
	var msg string

	if nextSend := state.buf.NextSendTime(); !nextSend.IsZero() && nextSend.After(now) {
		resetDuration = nextSend.Sub(now)
		msg = "waiting on batch.NextSendTime"
	}
	if resDelay := state.reservationToSend.DelayFrom(now); resDelay < resetDuration {
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
// This will ACK/NACK the LeasedBatch (once).
func (state *coordinatorState) handleResult(result *workerResult) {
	state.dbg("  GOT RESULT")

	evictMsg := ""
	if !result.batch.Live() {
		evictMsg = " (stale)"
	}

	if result.err == nil {
		state.dbg("    ACK" + evictMsg)
		result.batch.ACK()
		return
	}

	state.dbg("    ERR(%s)", result.err)
	if retry := state.opts.ErrorFn(result.batch.Batch, result.err); !retry {
		state.dbg("    NO RETRY" + evictMsg)
		result.batch.ACK()
		return
	}

	state.dbg("    NACK" + evictMsg)
	result.batch.NACK(state.ctx, result.err)
	return
}

// Returns true iff we're in the draining state and the buffer is now empty.
func (state *coordinatorState) drained() bool {
	return state.draining && state.buf.Len() == 0
}

// coordinator is the main goroutine for managing the state of the Channel.
// Exactly one coordinator() function runs per Channel. This coordinates (!!)
// all of the internal channels of the external Channel object in one big select
// loop.
func (c *Channel) coordinator(ctx context.Context, opts *Options, buf buffer.Buffer, send SendFn) {
	defer close(c.doneCh)

	state := c.mkCoordinatorState(ctx, opts, buf)
	defer close(state.batchCh)
	defer close(state.resultCh)

	defer state.timer.Stop()

	for i := 0; i < state.opts.MaxSenders; i++ {
		go state.worker(ctx, i, send)
	}

	for !state.drained() {
		state.dbg("LOOP: buf.Len[%d]", state.buf.Len())

		now := clock.Now(state.ctx)
		state.updateSendState(now)

		select {
		case <-state.ctx.Done():
			break

		case result := <-state.resultCh:
			state.handleResult(result)

		case result := <-state.getNextTimingEvent(now):
			if result.Incomplete() {
				break
			}
			state.dbg("  GOT TIMER WAKEUP")

		case itm, ok := <-state.getWorkChannel():
			if !ok {
				state.dbg("  GOT DRAIN")
				state.draining = true
				state.buf.Flush(state.ctx)
				continue
			}
			state.dbg("  GOT NEW DATA")
			if dropped := state.buf.AddNoBlock(state.ctx, itm); dropped != nil {
				state.opts.DropFn(dropped)
				if state.batchToSend != nil && state.batchToSend.Batch == dropped {
					state.batchToSend = nil
				}
			}

		case state.getSendChannel(now) <- state.batchToSend:
			state.dbg("  SENT BATCH")
			state.batchToSend = nil
			state.reservationToSend = nil
		}
	}

	state.dbg("DONE")
}
