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

// Channel holds a chan which you can push individual work items to.
type Channel struct {
	ctx  context.Context
	send SendFn

	itemCh chan interface{}

	batchCh  chan *buffer.LeasedBatch
	resultCh chan *workerResult

	doneCh chan struct{}

	buf  buffer.Buffer
	opts Options
}

// Chan returns an unbuffered channel which you can push single work items into.
// If this channel is closed, it means that the Channel is no longer accepting
// any new work items.
func (c *Channel) Chan() chan<- interface{} {
	return c.itemCh
}

// Close will prevent the Channel from accepting any new work items, flush its
// internal buffer, and will block until:
//
//   * All items have been sent/discarded; OR
//   * The Context is Done.
//
// Returns Context.Err(), or nil if the Channel finished its work normally.
func (c *Channel) Close(ctx context.Context) error {
	close(c.itemCh) // signals writers to stop and parentWorker to drain.

	// If there's any uncut batch, make it available to send. We don't want to
	// have to wait for the BatchDuration.
	c.buf.Flush(ctx)

	// Wait for either the context or the done signal to trigger.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.doneCh:
		return nil
	}
}

type workerResult struct {
	batch *buffer.LeasedBatch
	err   error
}

func (c *Channel) computeWorkInterval() time.Duration {
	return time.Duration(
		(float64(time.Second) / c.opts.MaxQPS) / float64(c.opts.MaxSenders))
}

func (c *Channel) worker() {
	workInterval := c.computeWorkInterval()

	timer := clock.NewTimer(c.ctx)
	defer timer.Stop()

	doWait := false
	for {
		if doWait {
			<-timer.GetC() // wakes up on ctx cancellation too
			doWait = false
		}
		select {
		case <-c.ctx.Done():
			return

		case batch := <-c.batchCh:
			start := clock.Now(c.ctx)
			err := c.send(c.ctx, batch.Batch)
			if toWait := workInterval - clock.Now(c.ctx).Sub(start); toWait > 0 {
				timer.Reset(toWait)
				doWait = true
			}
			c.resultCh <- &workerResult{batch, err}
		}
	}
}

func (c *Channel) parentWorker() {
	defer close(c.batchCh)
	defer close(c.resultCh)
	defer close(c.doneCh)

	var batchToSend *buffer.LeasedBatch

	timer := clock.NewTimer(c.ctx)
	defer timer.Stop()

	for i := 0; i < c.opts.MaxSenders; i++ {
		go c.worker()
	}

	draining := false

	for {
		if draining && c.buf.Len() == 0 {
			return
		}

		var sendChannel chan<- *buffer.LeasedBatch
		if batchToSend == nil {
			if batchToSend = c.buf.LeaseOne(c.ctx); batchToSend != nil {
				sendChannel = c.batchCh
			}
		}

		var timerC <-chan clock.TimerResult
		if nextSend := c.buf.NextSendTime(); !nextSend.IsZero() {
			now := clock.Now(c.ctx)
			if nextSend.After(now) {
				timer.Reset(nextSend.Sub(now))
				timerC = timer.GetC()
			}
		}

		var rChan <-chan interface{}
		if !draining && c.buf.CanAddItem() {
			rChan = c.itemCh
		}

		select {
		// UNCONDITIONAL CHANNELS
		case <-c.ctx.Done():
			return

		case result := <-c.resultCh:
			if result.err != nil {
				if c.opts.ErrorFn(c.ctx, result.batch.Batch, result.err) {
					result.batch.NACK(c.ctx, result.err)
				} else {
					result.batch.ACK()
				}
			}

		// CONDITIONAL CHANNELS
		case result := <-timerC:
			if result.Incomplete() {
				// context is canceled
				return
			}
			// buffer has something for us! catch it on the next loop.

		case itm, ok := <-rChan:
			if !ok {
				draining = true
				continue
			}
			c.buf.AddNoBlock(c.ctx, itm)

		case sendChannel <- batchToSend:
			// sweet, a worker picked it up
		}
	}
}

// SendFn is the function which does the work to actually transmit the Batch to
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
type SendFn func(ctx context.Context, data *buffer.Batch) error

// NewChannel produces a new Channel ready to listen and send work items.
//
// Args:
//   * `ctx` will be used for cancellation and logging.
//   * `send` is required, and defines the function to use to process Batches
//     of data.
//   * `opts` is optional (see Options for the defaults).
//
// The Channel must be Close()'d when you're done with it.
func NewChannel(ctx context.Context, send SendFn, opts *Options) (ret *Channel, err error) {
	if send == nil {
		err = errors.New("send is required: got nil")
		return
	}

	var optsCopy Options
	if opts != nil {
		optsCopy = *opts
	}
	if err = optsCopy.normalize(); err != nil {
		err = errors.Annotate(err, "normalizing dispatcher.Options").Err()
		return
	}

	buf, err := buffer.NewBuffer(&opts.Buffer)
	if err != nil {
		err = errors.Annotate(err, "allocating Buffer").Err()
		return
	}

	ret = &Channel{
		ctx:  ctx,
		send: send,

		itemCh:   make(chan interface{}),
		batchCh:  make(chan *buffer.LeasedBatch),
		resultCh: make(chan *workerResult),
		doneCh:   make(chan struct{}),

		buf:  buf,
		opts: optsCopy,
	}

	go ret.parentWorker()

	return
}
