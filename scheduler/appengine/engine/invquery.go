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
//  for len(results) != pageSize {
//    switch item, err := it.next(); {
//    case err != nil:
//      return nil, err // RPC error
//    case item == nil:
//      ...
//      return results, nil // fetched all available results
//    default:
//      results = append(results, item)
//    }
//  }
//  return results // fetched the full page
type invDatastoreIter struct {
	results chan *Invocation // receives results of the query
	done    chan struct{}    // closed when 'stop' is called
	err     error            // error status of the query, synchronized via 'results'
	stopped bool             // true if 'stop' was called
}

// start initiates the query.
//
// The iterator is initially positioned before the first item, so that a call
// to 'next' will return the first item.
func (it *invDatastoreIter) start(c context.Context, query *datastore.Query) {
	it.results = make(chan *Invocation)
	it.done = make(chan struct{})
	go func() {
		defer close(it.results)
		err := datastore.Run(c, query, func(obj *Invocation, cb datastore.CursorCB) error {
			select {
			case it.results <- obj:
				return nil
			case <-it.done:
				return datastore.Stop
			}
		})
		// Let 'next' and 'stop' know about the error. They look here if they
		// receive 'nil' from the results channel (which happens if it is closed).
		it.err = err
	}()
}

// next fetches the next query item if there's one.
//
// Returns (nil, nil) if all items has been successfully fetched. If the query
// failed, returns (nil, err).
func (it *invDatastoreIter) next() (*Invocation, error) {
	switch {
	case it.results == nil:
		panic("'next' is called before 'start'")
	case it.stopped:
		panic("'next' is called after 'stop'")
	}
	if inv, ok := <-it.results; ok {
		return inv, nil
	}
	return nil, it.err // 'it.err' is valid only after the channel is closed
}

// stop finishes the query, killing the internal goroutine.
//
// Once 'stop' is called, calls to 'next' are forbidden. It is OK to call
// 'stop' again though (it will return exact same value).
func (it *invDatastoreIter) stop() error {
	if !it.stopped {
		it.stopped = true
		close(it.done)         // signal the inner loop to wake up and exit
		for range it.results { // wait for the results channel to close
		}
	}
	return it.err
}
