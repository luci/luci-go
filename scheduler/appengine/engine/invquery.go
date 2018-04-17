// Copyright 2018 The LUCI Authors.
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

package engine

import (
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
)

// invDatastoreIter is a wrapper over datastore query that makes it look more
// like an iterator.
//
// Intended usage:
//
//  it.start(...)
//  defer it.stop()
//  for {
//    switch item, err := it.next(); {
//    case err != nil:
//      return nil, nil, err // RPC error
//    case item == nil:
//      ...
//      return results, nil, nil // fetched all available results
//    case item != nil:
//      results = append(results, item)
//    }
//    if len(results) == pageSize { // fetched the full page and have a cursor
//      cursor, err := it.stop()
//      ...
//      return results, cursor, err
//    }
//  }
type invDatastoreIter struct {
	started bool // true after first 'next' call
	stopped bool // true after 'stop' has been called

	resume  chan bool // unblocks inner datastore.Run iteration
	results chan bool // signals that some results are available

	// Variables used to communicate the state from the inner loop. Indirectly
	// synchronized via 'results' channel.
	inv    *Invocation
	cursor datastore.Cursor
	err    error
}

// start initiates the query.
//
// The iterator is initially positioned before the first item, so that a call
// to 'next' will return the first item.
func (it *invDatastoreIter) start(c context.Context, query *datastore.Query) {
	it.resume = make(chan bool)
	it.results = make(chan bool)
	go func() {
		err := datastore.Run(c, query, func(obj *Invocation, cb datastore.CursorCB) error {
			it.inv = obj
			it.results <- true
			if resume := <-it.resume; !resume {
				if it.cursor, it.err = cb(); it.err != nil {
					return it.err
				}
				return datastore.Stop
			}
			return nil
		})
		// Signal that we are done returning results, they will not change.
		it.inv, it.err = nil, err
		close(it.results)
		// Drain 'resume' channel until it is closed by 'stop'. We need this to
		// unblock senders on this channel.
		for range it.resume {
		}
	}()
}

// next fetches the next query item if there's one.
//
// Returns (nil, nil) if all items has been successfully fetched. If the query
// failed, returns (nil, err).
func (it *invDatastoreIter) next() (*Invocation, error) {
	switch {
	case it.resume == nil:
		panic("'next' is called before 'start'")
	case it.stopped:
		panic("'next' is called after 'stop'")
	}
	// If it's the first 'next' call ever, there's no need to wake up the loop
	// body, since it is not sleeping yet (it falls asleep only after it fetches
	// the item, i.e. after <-results is unblocked).
	if !it.started {
		it.started = true
	} else {
		it.resume <- true // tell the loop to spin one iteration
	}
	<-it.results // wait until 'it.inv' and 'it.err' are populated
	return it.inv, it.err
}

// stop ends the query and returns the cursor pointing to the item after the
// current one, if there were some results we haven't fetched yet, or nil cursor
// if we fetched all available results.
//
// 'stop' must be called to releases the resources associated with the query, so
// call it even if you are not interested in the cursor.
//
// Once 'stop' is called, calls to 'next' are forbidden. It is OK to call
// 'stop' again though (it will return exact same values).
func (it *invDatastoreIter) stop() (datastore.Cursor, error) {
	if !it.stopped {
		it.stopped = true
		close(it.resume) // signal the inner loop to wake up and exit
		<-it.results     // wait for the cursor and the error
	}
	return it.cursor, it.err
}
