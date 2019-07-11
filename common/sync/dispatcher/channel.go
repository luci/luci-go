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
)

// Channel holds a chan which you can push individual work items to.
type Channel struct {
	// WriteChan is an unbuffered channel which you can push single work items
	// into. If this channel is closed, it means that the Channel is no longer
	// accepting any new work items.
	WriteChan chan<- interface{}
}

// Close will prevent the Channel from accepting any new work items, and will
// block until:
//
//   * All items have been sent/discarded; OR
//   * The Context passed to NewChannel is Done.
//
// Returns Context.Err() (i.e. this only returns an error if the Context was
// canceled).
func (*Channel) Close() error {
	panic("not implemented")
}

// NewChannel produces a new Channel ready to listen and send work items.
//
// Args:
//   * `ctx` will be used for cancellation and logging.
//   * `opts` is optional (see Options for the defaults).
//
// The Channel must be Close()'d when you're done with it.
func NewChannel(ctx context.Context, opts Options) (*Channel, error) {
	if err := opts.normalize(); err != nil {
		return nil, errors.Annotate(err, "normalizing dispatcher.Options").Err()
	}
	panic("not implemented")
}
