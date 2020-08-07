// Copyright 2020 The LUCI Authors.
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

// Package workset contains a synchronized work queue implementation used by
// inproc sweeper.
package workset

import (
	"context"
	"sync"
)

// Item is one item that when processed may result in more items.
type Item interface{}

// WorkSet represents a set of some available work items.
//
// Each item, when processed, may produce more work items. Items in the set
// are deliberately unordered. Item implementations should also be hashable,
// since they'll be added to a map.
type WorkSet struct {
	m        sync.Mutex
	work     map[Item]struct{} // available queued work
	inflight int               // how many workers are busy now
	waiting  chan struct{}     // signaled to wake up all waiters
}

// New creates a new WorkSet and seeds it with the initial set of work items.
func New(work []Item) *WorkSet {
	set := make(map[Item]struct{}, len(work))
	for _, w := range work {
		set[w] = struct{}{}
	}
	return &WorkSet{work: set}
}

// Pop blocks until there's some work item and then pops it.
//
// Once it happens, it also increments "somebody is working" counter. The worker
// should eventually call `done(<more work items>)` to acknowledge that it has
// finished processing the item.
//
// If there's no available work items and no one is working (and thus no new
// items may ever appear), returns nil.
//
// Returns nil as well if the context is canceled.
func (ws *WorkSet) Pop(ctx context.Context) (item Item, done func([]Item)) {
	ws.m.Lock()
	defer ws.m.Unlock() // we'll also be unlocking it in the middle of the loop

	// If there's no work available, but someone may produce it, block. Note that
	// this is essentially a conditional variable (with broadcast) implemented on
	// top of channels for compatibility with ctx.Done().
	for len(ws.work) == 0 && ws.inflight > 0 && ctx.Err() == nil {
		if ws.waiting == nil {
			ws.waiting = make(chan struct{})
		}
		waiting := ws.waiting // will be closed in done(...) to wake everyone

		// Release the lock while we are waiting, to let others reach this stage
		// as well.
		ws.m.Unlock()
		select {
		case <-waiting:
		case <-ctx.Done():
		}
		ws.m.Lock()
	}

	if len(ws.work) == 0 {
		return nil, nil // either the workset is empty or the context is done
	}

	ws.inflight++

	// Pop a random work item.
	for item = range ws.work {
		break
	}
	delete(ws.work, item)

	called := false
	return item, func(more []Item) {
		ws.m.Lock()
		defer ws.m.Unlock()

		if called {
			panic("`done` callback called twice")
		}
		called = true
		ws.inflight--

		// Enqueue all follow up work.
		for _, w := range more {
			ws.work[w] = struct{}{}
		}

		// If conditional variable's condition is signaled, wake everyone up.
		if ws.waiting != nil && (len(ws.work) != 0 || ws.inflight == 0) {
			close(ws.waiting)
			ws.waiting = nil
		}
	}
}
