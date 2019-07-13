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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/common/sync/dispatcher/token"
)

// Channel holds a chan which you can push individual work items to.
type Channel struct {
	ctx  context.Context
	send SendFn

	closedCh chan struct{}

	itemCh chan interface{}

	batchCh  chan *buffer.LeasedBatch
	resultCh chan *workerResult

	doneCh chan struct{}

	buf  buffer.Buffer
	qps  *token.Bucket
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
	itemCh := c.itemCh // dereference outside of recover function
	func() {
		// Oof, this sucks; however, the 'close' behavior for itemCh is well
		// defined for Channel and it's legitimate to close it from different
		// goroutines.
		defer func() { recover() }()
		close(itemCh) // signals writers to stop and parentWorker to drain.
	}()

	// If there's any uncut batch, make it available to send. We don't want to
	// have to wait for the BatchDuration. Calling this multiple times is safe
	// because itemCh is already closed and so the second+ call will be a noop.
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

func (c *Channel) worker(id int) {
	ctx := logging.SetField(c.ctx, "dispatcher.worker", id)

	for {
		logging.Debugf(ctx, "getting work token")
		select {
		case <-ctx.Done():
		case <-c.qps.C:
		}

		logging.Debugf(ctx, "getting work")
		select {
		case <-ctx.Done():
			return

		case batch, ok := <-c.batchCh:
			if !ok {
				logging.Debugf(ctx, "dead")
				return
			}
			logging.Debugf(ctx, "processing batch: len == %d", len(batch.Data))
			rslt := &workerResult{batch: batch}
			if rslt.err = c.send(batch.Batch); rslt.err != nil {
				logging.Debugf(ctx, "  err: %s", rslt.err)
			}
			logging.Debugf(ctx, "  send result")
			c.resultCh <- rslt
		}
	}
}

func (c *Channel) parentWorker() {
	defer close(c.batchCh)
	defer close(c.resultCh)
	defer close(c.doneCh)
	defer c.qps.Close()

	var batchToSend *buffer.LeasedBatch

	ctx := logging.SetField(c.ctx, "dispatcher.parentWorker", true)

	timer := clock.NewTimer(clock.Tag(ctx, "parent"))
	defer timer.Stop()

	for i := 0; i < c.opts.MaxSenders; i++ {
		go c.worker(i)
	}

	draining := false

	for {
		if draining && c.buf.Len() == 0 {
			logging.Debugf(ctx, "DONE")
			return
		}

		logging.Debugf(ctx, "WAKE {")

		var sendChannel chan<- *buffer.LeasedBatch
		if batchToSend == nil {
			batchToSend = c.buf.LeaseOne(ctx)
		}
		if batchToSend != nil {
			logging.Debugf(ctx, "  waiting on send")
			sendChannel = c.batchCh
		}

		var timerC <-chan clock.TimerResult
		if nextSend := c.buf.NextSendTime(); !nextSend.IsZero() {
			now := clock.Now(ctx)
			if nextSend.After(now) {
				duration := nextSend.Sub(now)
				logging.Debugf(ctx, "  waiting on batch (%s) : %s", duration, nextSend)
				timer.Reset(duration)
				timerC = timer.GetC()
			}
		}

		var rChan <-chan interface{}
		if !draining && c.buf.CanAddItem() {
			logging.Debugf(ctx, "  waiting on work")
			rChan = c.itemCh
		}

		logging.Debugf(ctx, "}  BLOCK:")
		select {
		// UNCONDITIONAL CHANNELS
		case <-ctx.Done():
			logging.Debugf(ctx, "  GOT CTX DONE")
			return

		case result := <-c.resultCh:
			logging.Debugf(ctx, "  GOT RESULT")
			if result.err != nil {
				logging.Debugf(ctx, "    ERR(%s)", result.err)
				if c.opts.ErrorFn(result.batch.Batch, result.err) {
					logging.Debugf(ctx, "    NACK @ %s", clock.Now(ctx))
					dropped, requeued := result.batch.NACK(ctx, result.err)
					if dropped != nil {
						c.opts.DropFn(dropped)
					}
					if !requeued {
						c.opts.DropFn(result.batch.Batch)
					}
					continue
				} else {
					c.opts.DropFn(result.batch.Batch)
				}
			}
			logging.Debugf(ctx, "    ACK")
			result.batch.ACK()

		// CONDITIONAL CHANNELS
		case result := <-timerC:
			if result.Incomplete() {
				// context is canceled
				logging.Debugf(ctx, "  GOT CTX CANCEL")
				return
			}
			// buffer has something for us! catch it on the next loop.
			logging.Debugf(ctx, "  GOT BUFFER TIMEOUT")

		case itm, ok := <-rChan:
			if !ok {
				draining = true
				logging.Debugf(ctx, "  GOT DRAIN")
				continue
			}
			logging.Debugf(ctx, "  GOT WORK")
			c.buf.AddNoBlock(ctx, itm)

		case sendChannel <- batchToSend:
			logging.Debugf(ctx, "  SENT WORK")
			batchToSend = nil
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
// The SendFn should be bound to this Channel's Context; if the Channel's
// Context is Cancel'd, SendFn should terminate. We don't pass it as part of
// SendFn's signature in case SendFn needs to be bound to a derived Context.
//
// Non-nil errors returned by this function will be handled by ErrorFn.
type SendFn func(data *buffer.Batch) error

// NewChannel produces a new Channel ready to listen and send work items.
//
// Args:
//   * `ctx` will be used for cancellation and logging.
//   * `send` is required, and defines the function to use to process Batches
//     of data.
//   * `opts` is optional (see Options for the defaults).
//
// The Channel must be Close()'d when you're done with it.
func NewChannel(ctx context.Context, opts *Options, send SendFn) (ret *Channel, err error) {
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
	if optsCopy.ErrorFn == nil {
		optsCopy.ErrorFn = defaultErrorFnFactory(ctx)
	}
	if optsCopy.DropFn == nil {
		optsCopy.DropFn = defaultDropFnFactory(ctx, optsCopy.Buffer.FullBehavior)
	}

	buf, err := buffer.NewBuffer(&optsCopy.Buffer)
	if err != nil {
		err = errors.Annotate(err, "allocating Buffer").Err()
		return
	}

	var qps *token.Bucket
	if optsCopy.MaxQPS == -1 {
		qps = token.NewBucket(ctx, 0, -1)
	} else {
		qps = token.NewBucket(ctx, optsCopy.MaxSenders, optsCopy.MaxQPS)
	}

	ret = &Channel{
		ctx:  ctx,
		send: send,

		itemCh:   make(chan interface{}),
		batchCh:  make(chan *buffer.LeasedBatch),
		resultCh: make(chan *workerResult),
		doneCh:   make(chan struct{}),

		qps:  qps,
		buf:  buf,
		opts: optsCopy,
	}

	go ret.parentWorker()

	return
}
