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

type coordinatorState struct {
	opts Options
	buf  *buffer.Buffer

	itemCh  <-chan interface{}
	drainCh chan<- struct{}

	resultCh chan workerResult

	// Used as a wake-up timer for the coordinator to wake itself up when the
	// buffer will have a batch available due to buffer timeout and/or qps limiter.
	timer clock.Timer

	// true iff we're supposed to process the rest of our work, but no new work
	// will come in.
	draining bool
}

type workerResult struct {
	batch *buffer.Batch
	err   error
}

func (state *coordinatorState) dbg(msg string, args ...interface{}) {
	if state.opts.testingDbg != nil {
		state.opts.testingDbg(msg, args...)
	}
}

func (state *coordinatorState) sendBatches(ctx context.Context, now time.Time, send SendFn) time.Duration {
	for {
		// See if we're permitted to send.
		res := state.opts.QPSLimit.ReserveN(now, 1)
		if !res.OK() {
			panic(errors.New(
				"impossible; Options.QPSLimit is guaranteed to have Inf rate or burst >= 1"))
		}
		if delay := res.DelayFrom(now); delay != 0 {
			// We have to wait until the next send token is available. Cancel the
			// reservation for now, since we're going to wait via getNextTimingEvent.
			res.CancelAt(now)
			return delay
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
				case <-ctx.Done():
				case state.resultCh <- rslt:
				}
			}()
		} else {
			// Otherwise we're done sending batches for now. Cancel the reservation,
			// since we can't use it.
			res.CancelAt(now)
			return 0
		}
	}
}

// getNextTimingEvent returns a clock.Timer channel which will activate when the
// later of the following happen:
//   * buffer.NextSendTime
//   * nextQPSToken
func (state *coordinatorState) getNextTimingEvent(now time.Time, nextQPSToken time.Duration) <-chan clock.TimerResult {
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
		return state.timer.GetC()
	}
	return nil
}

// getWorkChannel returns a channel to receive an individual work item on (from
// our client) if our buffer is willing to accept additional work items.
//
// Otherwise returns nil.
func (state *coordinatorState) getWorkChannel() <-chan interface{} {
	if !state.draining && state.buf.CanAddItem() {
		state.dbg("  |waiting on new data")
		return state.itemCh
	}
	return nil
}

// handleResult is invoked once for each workerResult returned to the
// coordinator from a worker.
//
// This will ACK/NACK the Batch (once).
func (state *coordinatorState) handleResult(ctx context.Context, result workerResult) {
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
	state.buf.NACK(ctx, result.err, result.batch)
	return
}

// coordinator is the main goroutine for managing the state of the Channel.
// Exactly one coordinator() function runs per Channel. This coordinates (!!)
// all of the internal channels of the external Channel object in one big select
// loop.
func (state *coordinatorState) run(ctx context.Context, send SendFn) {
	defer close(state.drainCh)
	defer close(state.resultCh)
	defer state.timer.Stop()

loop:
	for {
		bufStats := state.buf.Stats()
		if state.draining && bufStats.Empty() {
			break loop
		}
		state.dbg("LOOP (draining: %t): buf.Stats[%+v]", state.draining, bufStats)

		now := clock.Now(ctx)

		resDelay := state.sendBatches(ctx, now, send)

		select {
		case <-ctx.Done():
			break loop

		case result := <-state.resultCh:
			state.handleResult(ctx, result)

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
			state.sendBatches(ctx, result.Time, send)
		}
	}

	state.dbg("DONE")
}
