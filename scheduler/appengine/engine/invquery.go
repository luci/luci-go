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

// invDatastoreQuery is a wrapper over datastore query that makes it look more
// like an iterator.
//
// Intended usage:
//
//  q.start(...)
//  defer q.stop()
//  for {
//    switch item, err := q.next(); {
//    case err != nil:
//      return nil, nil, err // RCP error
//    case item == nil:
//      ...
//      return results, nil, nil // fetched all available results
//    case item != nil:
//      results = append(results, item)
//    }
//    if len(results) == pageSize { // fetched the full page and have a cursor
//      cursor, err := q.stop()
//      ...
//      return results, cursor, err
//    }
//  }
type invDatastoreQuery struct {
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
// The iterator is initially positioned before the first result.
func (q *invDatastoreQuery) start(c context.Context, query *datastore.Query) {
	q.resume = make(chan bool)
	q.results = make(chan bool)
	go func() {
		err := datastore.Run(c, query, func(obj *Invocation, cb datastore.CursorCB) error {
			q.inv, q.cursor, q.err = obj, nil, nil
			q.results <- true
			if resume := <-q.resume; !resume {
				if q.cursor, q.err = cb(); q.err != nil {
					return q.err
				}
				return datastore.Stop
			}
			return nil
		})
		// Signal that we are done returning results, they will not change.
		q.inv, q.err = nil, err
		close(q.results)
		// Drain 'resume' channel until it is closed by 'stop'. We need this to
		// unblock senders on this channel.
		for range q.resume {
		}
	}()
}

// next fetches the next query item if there's one.
//
// Returns (nil, nil) if all items has been successfully fetched. If the query
// failed, returns (nil, err).
func (q *invDatastoreQuery) next() (*Invocation, error) {
	if q.stopped {
		panic("'next' is called after 'stop'")
	}
	// If it's the first 'next' call ever, there's no need to wake up the loop
	// body, since it is not sleeping yet (it falls asleep only after it fetches
	// the item, i.e. after <-results is unblocked).
	if !q.started {
		q.started = true
	} else {
		q.resume <- true // tell the loop to spin one iteration
	}
	<-q.results // wait until 'q.inv' and 'q.err' are populated
	return q.inv, q.err
}

// stop ends the query and returns the cursor pointing to the item after the
// current one if there were some results we haven't fetched yet.
//
// Once 'stop' is called, calls to 'next' are forbidden. It is OK to call
// 'stop' again though (it will return exact same values).
//
// 'stop' must be called to releases the resources associated with the query.
func (q *invDatastoreQuery) stop() (datastore.Cursor, error) {
	if !q.stopped {
		q.stopped = true
		close(q.resume) // signal the inner loop to wake up and exit
		<-q.results     // wait for the cursor
	}
	return q.cursor, q.err
}
