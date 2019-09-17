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
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

// Channel holds a chan which you can push individual work items to.
type Channel struct {
	// C is an unbuffered channel which you can push single work items into.
	//
	// Close this to shut down the Channel.
	C chan<- interface{}

	// DrainC will unblock when this Channel is closed/canceled and fully drained.
	DrainC <-chan struct{}
}

// Close is a convenience function which closes C (and swallows panic if
// already closed).
func (c Channel) Close() {
	defer func() { recover() }()
	close(c.C)
}

// CloseAndDrain is a convenience function which closes C (and swallows panic if
// already closed) and then blocks on DrainC/ctx.Done().
func (c Channel) CloseAndDrain(ctx context.Context) {
	c.Close()
	select {
	case <-ctx.Done():
	case <-c.DrainC:
	}
}

// IsDrained returns true iff the Channel is closed and drained.
func (c Channel) IsDrained() bool {
	select {
	case <-c.DrainC:
		return true
	default:
		return false
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
//
// TODO(iannucci): Remove `ctx` as input from NewChannel, and as a source of
// cancelation. Resoning:
//   * We must provide Channel.C for efficient use of the Channel (i.e. so it
//     can be used in select statements.
//   * We must correctly support closure of Channel.C; there's no way to provide
//     a send channel without restricting closure on that channel.
//   * Accepting cancelation via `ctx` means that we have TWO non-equivalent
//     ways to teardown... ick.
//   * Due to constraints of the go race detector, it's not possible for Channel
//     to close Channel.C when ctx is canceled; Apparently this counts as
//     a read/write action, not just a write action.
//   * Leaving C open, but shutting down the coordinator goroutine, means that
//     users of Channel could be blocked forever when they should
//     panic/stop/whatever.
//   * Adding an infinite consumer loop on Channel.C means that producers could
//     produce at an infinite rate... also not great.
//   * The only thing that Channel uses ctx for is:
//     * logging in the default Error/DropFn's
//     * the retry.Iterator from Buffer
//     * clock/timer stuff for testing; external packages can remove the
//       reliance on timing information in tests by setting an infinite rate
//       via Options, so external users shouldn't be attempting to manipulate
//       time within the Channel.
//     * this extra cancelation problem/feature
//
// Plan: Remove non-cancelation uses of ctx, then remove ctx from the external
// view. The trickiest dependency is retry.Next, which, IMO is a poor use of
// context anyway; retry.Next is bound to an Iterator struct, producecd from
// a retry.Factory; There's plenty of opportunity for the user to bind a context
// that they want from any of those places.
func NewChannel(ctx context.Context, opts *Options, send SendFn) (Channel, error) {
	if send == nil {
		return Channel{}, errors.New("send is required: got nil")
	}

	var optsCopy Options
	if opts != nil {
		optsCopy = *opts
	}

	buf, err := buffer.NewBuffer(&optsCopy.Buffer)
	if err != nil {
		return Channel{}, errors.Annotate(err, "allocating Buffer").Err()
	}

	if err = optsCopy.normalize(ctx); err != nil {
		return Channel{}, errors.Annotate(err, "normalizing dispatcher.Options").Err()
	}

	itemCh := make(chan interface{})
	drainCh := make(chan struct{})

	cstate := coordinatorState{
		opts:    optsCopy,
		buf:     buf,
		itemCh:  itemCh,
		drainCh: drainCh,

		resultCh: make(chan workerResult),

		timer: clock.NewTimer(clock.Tag(ctx, "coordinator")),
	}

	go cstate.run(ctx, send)
	return Channel{C: itemCh, DrainC: drainCh}, nil
}
