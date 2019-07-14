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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

type coordinatorState struct {
	// The Channel we belong to.
	channel *Channel

	ctx context.Context

	// Used as a wake-up timer for the coordinator to wake itself up when the
	// buffer will have a batch available due to timeout.
	bufferReadyTimer clock.Timer

	// true iff we're supposed to process the rest of or work, but no new work
	// will come in.
	draining bool

	// The batch we'll send to the workers next. Because the coordinator loop only
	// does one action per loop, this may persist for multiple loops. If this is
	// nil, may be populated by getBatchSendChannel.
	batchToSend *buffer.LeasedBatch
}

func (c *Channel) mkCoordinatorState() coordinatorState {
	ctx := logging.SetField(c.ctx, "dispatcher.coordinator", true)
	return coordinatorState{
		channel:          c,
		ctx:              ctx,
		bufferReadyTimer: clock.NewTimer(clock.Tag(ctx, "coordinator")),
	}
}

func (p *coordinatorState) dbg(fmt string, args ...interface{}) {
	logging.Debugf(p.ctx, fmt, args...)
}

// getBatchSendChannel returns a channel to send a LeasedBatch to a worker if:
//   * we have a leased batch already (in batchToSend) OR
//   * our buffer has a new batch to lease.
//
// If the buffer has a new batch, this updates batchToSend. The implication is
// that batchToSend will always be populated if this method returns non-nil.
//
// If there's no batch to send, this returns nil.
func (p *coordinatorState) getBatchSendChannel() (send chan<- *buffer.LeasedBatch) {
	if p.batchToSend == nil {
		p.batchToSend = p.channel.buf.LeaseOne(p.ctx)
	}
	if p.batchToSend != nil {
		p.dbg("  |waiting on send")
		send = p.channel.batchCh
	}
	return send
}

// getBufferReadyTimer returns a clock.Timer channel if the buffer has
// a 'NextSendTime' in the future.
//
// Otherwise returns nil.
func (p *coordinatorState) getBufferReadyTimer() (bufReady <-chan clock.TimerResult) {
	if nextSend := p.channel.buf.NextSendTime(); !nextSend.IsZero() {
		now := clock.Now(p.ctx)
		if nextSend.After(now) {
			duration := nextSend.Sub(now)
			p.dbg("  |waiting on batch (%s) : %s", duration, nextSend)
			p.bufferReadyTimer.Reset(duration)
			bufReady = p.bufferReadyTimer.GetC()
		}
	}
	return
}

// getWorkChannel returns a channel to recieve an individual work item on (from
// our client) if our buffer is willing to accept additional work items.
//
// Otherwise returns nil.
func (p *coordinatorState) getWorkChannel() (work <-chan interface{}) {
	if !p.draining && p.channel.buf.CanAddItem() {
		p.dbg("  |waiting on work")
		work = p.channel.itemCh
	}
	return
}

// handleResult is invoked once for each workerResult returned to the
// coordinator from a worker.
//
// This invokes Options.ErrorFn and/or Options.DropFn, and will also ACK/NACK
// the LeasedBatch (once).
func (p *coordinatorState) handleResult(result *workerResult) {
	p.dbg("  GOT RESULT")
	if result.err != nil {
		p.dbg("    ERR(%s)", result.err)
		if p.channel.opts.ErrorFn(result.batch.Batch, result.err) {
			p.dbg("    NACK @ %s", clock.Now(p.ctx))
			dropped, requeued := result.batch.NACK(p.ctx, result.err)
			if dropped != nil {
				p.channel.opts.DropFn(dropped)
			}
			if !requeued {
				p.channel.opts.DropFn(result.batch.Batch)
			}
			return
		}
		p.channel.opts.DropFn(result.batch.Batch)
	}
	p.dbg("    ACK")
	result.batch.ACK()
}

// Returns true iff we're in the draining state and the buffer is now empty.
func (p *coordinatorState) drained() bool {
	return p.draining && p.channel.buf.Len() == 0
}

// coordinator is the main goroutine for managing the state of the Channel.
// Exactly one coordinator() function runs per Channel. This coordinates (!!)
// all of the internal channels of the external Channel object in one big select
// loop.
func (c *Channel) coordinator() {
	defer close(c.batchCh)
	defer close(c.resultCh)
	defer close(c.doneCh)
	defer c.qpsStop()

	state := c.mkCoordinatorState()

	defer state.bufferReadyTimer.Stop()

	for i := 0; i < c.opts.MaxSenders; i++ {
		go c.worker(i)
	}

	for !state.drained() {
		state.dbg("LOOP")
		select {
		case <-state.ctx.Done():
			break

		case result := <-c.resultCh:
			state.handleResult(result)

		case result := <-state.getBufferReadyTimer():
			if result.Incomplete() {
				break
			}
			// buffer has something for us! catch it on the next loop.
			state.dbg("  GOT BUFFER TIMEOUT")

		case itm, ok := <-state.getWorkChannel():
			if !ok {
				state.dbg("  GOT DRAIN")
				state.draining = true
				continue
			}
			state.dbg("  GOT WORK")
			c.buf.AddNoBlock(state.ctx, itm)

		case state.getBatchSendChannel() <- state.batchToSend:
			// getBatchSendChannel updates state.batchToSend before the select
			// statement as a side effect.
			state.dbg("  SENT WORK")
			state.batchToSend = nil
		}
	}

	state.dbg("DONE")
}
