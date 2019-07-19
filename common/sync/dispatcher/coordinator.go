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
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

type coordinatorState struct {
	opts Options
	buf  *buffer.Buffer

	itemCh  <-chan interface{}
	drainCh chan<- struct{}

	ctx context.Context

	isDbg bool
	dbg   func(fmt string, args ...interface{})

	resultCh chan workerResult

	// Used as a wake-up timer for the coordinator to wake itself up when the
	// buffer will have a batch available due to buffer timeout and/or qps limiter.
	timer clock.Timer

	// true iff we're supposed to process the rest of or work, but no new work
	// will come in.
	draining bool
}

type workerResult struct {
	batch *buffer.Batch
	err   error
}

func (state *coordinatorState) sendBatches(now time.Time, send SendFn) (delay time.Duration) {
	for {
		// See if we're permitted to send.
		res := state.opts.QPSLimit.ReserveN(now, 1)
		if !res.OK() {
			panic("impossible")
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
			go func() {
				rslt := workerResult{
					batch: batchToSend,
					err:   send(batchToSend),
				}
				select {
				case <-state.ctx.Done():
				case state.resultCh <- rslt:
				}
			}()
		} else {
			// Otherwise we're done sending batches for now. Cancel the reservation,
			// since we can't use it.
			res.CancelAt(now)
			return
		}
	}
}

// getNextTimingEvent returns a clock.Timer channel which will activate when the
// later of the following happen:
//   * buffer.NextSendTime
//   * nextQPSToken
func (state *coordinatorState) getNextTimingEvent(now time.Time, nextQPSToken time.Duration) (timeEvent <-chan clock.TimerResult) {
	var resetDuration time.Duration
	var msg string

	if nextSend := state.buf.NextSendTime(); !nextSend.IsZero() && nextSend.After(now) {
		resetDuration = nextSend.Sub(now)
		msg = "waiting on batch.NextSendTime"
	}

	if nextQPSToken > resetDuration {
		resetDuration = nextQPSToken
		msg = "waiting on QPS limit"
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
		work = state.itemCh
	}
	return
}

// handleResult is invoked once for each workerResult returned to the
// coordinator from a worker.
//
// This will ACK/NACK the Batch (once).
func (state *coordinatorState) handleResult(result workerResult) {
	state.dbg("  GOT RESULT")

	if result.err == nil {
		state.dbg("    ACK")
		state.buf.ACK(result.batch)
		return
	}

	state.dbg("    ERR(%s)", result.err)
	if retry := state.opts.ErrorFn(result.batch, result.err); !retry {
		state.dbg("    NO RETRY (dropping batch)")
		state.opts.DropFn(result.batch)
		state.buf.ACK(result.batch)
		return
	}

	state.dbg("    NACK")
	state.buf.NACK(state.ctx, result.err, result.batch)
	return
}

// coordinator is the main goroutine for managing the state of the Channel.
// Exactly one coordinator() function runs per Channel. This coordinates (!!)
// all of the internal channels of the external Channel object in one big select
// loop.
func (state *coordinatorState) run(send SendFn) {
	defer close(state.drainCh)
	defer close(state.resultCh)
	defer state.timer.Stop()

loop:
	for !state.draining || !state.buf.Empty() {
		if state.isDbg {
			// avoid Stats() call in non-debug mode.
			state.dbg("LOOP (draining: %t): buf.Stats[%+v]", state.draining, state.buf.Stats())
		}

		now := clock.Now(state.ctx)

		resDelay := state.sendBatches(now, send)

		select {
		case <-state.ctx.Done():
			break loop

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
				state.dbg("    dropped batch")
				state.opts.DropFn(dropped)
			}

		case result := <-state.getNextTimingEvent(now, resDelay):
			if result.Incomplete() {
				break loop
			}
			state.dbg("  GOT TIMER WAKEUP")
			// opportunistically attempt to send batches; either a new batch is ready
			// to be cut or the qps timer is up. This lowers the upper bound variance
			// and gets a bit closer to the QPS target.
			state.sendBatches(result.Time, send)
		}
	}

	state.dbg("DONE")
}
