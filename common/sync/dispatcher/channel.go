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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
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
		close(itemCh) // signals writers to stop and coordinator to drain.
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

	buf, err := buffer.NewBuffer(&optsCopy.Buffer)
	if err != nil {
		err = errors.Annotate(err, "allocating Buffer").Err()
		return
	}

	if err = optsCopy.normalize(ctx); err != nil {
		err = errors.Annotate(err, "normalizing dispatcher.Options").Err()
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

	go ret.coordinator()

	return
}
