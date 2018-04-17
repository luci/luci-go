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
	"sort"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// invQuery abstracts a query that fetches invocations in order of their IDs,
// smallest to largest.
//
// Think of it as a pointer to the head of the query, that can be advanced on
// demand.
type invQuery interface {
	// peek returns the invocation the query currently points to.
	//
	// It is fetched when 'advance' is called. A first call to 'peek' may also
	// initiate a fetch (to grab the first ever item).
	//
	// Returns nil if there's no more invocations to fetch. Returns an error if
	// the fetch failed.
	peek() (*Invocation, error)

	// advance fetches the next invocation to be returned by 'peek'.
	//
	// Returns an error if this operation fails. Reaching the end of the results
	// is not an error. If this happened, next 'peek' returns nil, and keeps
	// returning nil forever.
	advance() error
}

// mergeInvQueries merges results of multiple queries together.
//
// It picks smallest IDs first.
func mergeInvQueries(qs []invQuery, limit int) (out []*Invocation, all bool, err error) {
	out = make([]*Invocation, 0, limit)

	for len(out) != limit {
		// Find the smallest invocation from heads of all queries.
		var smallest *Invocation
		for _, q := range qs {
			inv, err := q.peek()
			if err != nil {
				return nil, false, err
			}
			if inv == nil {
				continue // exhausted results of this query
			}
			if smallest == nil || inv.ID < smallest.ID {
				smallest = inv
			}
		}

		if smallest == nil {
			all = true
			break // exhausted results of all queries
		}
		out = append(out, smallest)

		// There may be duplicates in the queries, so need to pop the consumed
		// invocation from all queries.
		for _, q := range qs {
			for {
				inv, err := q.peek()
				if err != nil {
					return nil, false, err
				}
				if inv == nil || inv.ID > smallest.ID {
					break // found something larger at the head
				}
				if err := q.advance(); err != nil {
					return nil, false, err
				}
			}
		}
	}

	return
}

////////////////////////////////////////////////////////////////////////////////
// List based queries.

// invListQuery implements invQuery on top of a sorted list of Invocations.
//
// The list is assumed to be sorted by IDs in smallest-to-largest order. This is
// also the order in which invocations will be returned.
type invListQuery struct {
	invs []*Invocation
	cur  int
}

func (q *invListQuery) peek() (*Invocation, error) {
	if q.cur == len(q.invs) {
		return nil, nil
	}
	return q.invs[q.cur], nil
}

func (q *invListQuery) advance() error {
	if q.cur < len(q.invs) {
		q.cur++
	}
	return nil
}

// activeInvQuery returns invQuery that emits active invocations, as fetched
// from the job.ActiveInvocations field.
//
// Smallest IDs are returned first. IDs smaller or equal than lastReturned are
// skipped (this is used for pagination).
func activeInvQuery(c context.Context, j *Job, lastReturned int64) *invListQuery {
	var invs []*Invocation
	for _, id := range j.ActiveInvocations {
		if id > lastReturned {
			invs = append(invs, &Invocation{ID: id})
		}
	}
	sort.Slice(invs, func(l, r int) bool { return invs[l].ID < invs[r].ID })
	return &invListQuery{invs: invs}
}

// recentInvQuery returns invQuery that emits recently finished invocations, as
// fetched from the job.FinishedInvocationsRaw field.
//
// Smallest IDs are returned first. IDs smaller or equal than lastReturned are
// skipped (this is used for pagination).
func recentInvQuery(c context.Context, j *Job, lastReturned int64) *invListQuery {
	finished, err := filteredFinishedInvs(
		j.FinishedInvocationsRaw, clock.Now(c).Add(-FinishedInvocationsHorizon))
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to unmarshal FinishedInvocationsRaw, skipping")
		return &invListQuery{}
	}

	var invs []*Invocation
	for _, inv := range finished {
		if inv.InvocationId > lastReturned {
			invs = append(invs, &Invocation{ID: inv.InvocationId})
		}
	}
	sort.Slice(invs, func(l, r int) bool { return invs[l].ID < invs[r].ID })
	return &invListQuery{invs: invs}
}

////////////////////////////////////////////////////////////////////////////////
// Datastore based queries.

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

// invDatastoreQuery implements invQuery on top of a datastore iterator.
//
// The datastore query results are expected to be sorted by IDs in
// smallest-to-largest order. This is also the order in which invocations will
// be returned.
type invDatastoreQuery struct {
	done bool // true if fetched everything we could

	iter   *invDatastoreIter // iterator positioned before the next result
	cursor datastore.Cursor  // the initial cursor

	advanced bool        // true if 'advance' was called at least once
	head     *Invocation // value to return in peek() or nil if haven't fetched yet
	err      error       // non-nil if the last fetch failed
}

func (q *invDatastoreQuery) peek() (*Invocation, error) {
	if q.done || q.err != nil {
		return nil, q.err // in a final non-advancable state
	}
	if q.head == nil {
		q.advance() // need to fetch the first item ever
	}
	return q.head, q.err
}

func (q *invDatastoreQuery) advance() error {
	if q.done || q.err != nil {
		return q.err // in a final non-advancable state
	}
	q.advanced = true
	q.head, q.err = q.iter.next()
	if q.head == nil && q.err == nil {
		q.done = true
	}
	return q.err
}

func (q *invDatastoreQuery) updateCursor(c *invocationsCursor) error {
	switch {
	case q.err != nil:
		return q.err
	case q.done:
		c.FinishedCursor = nil
		c.NextFinished = -1
		return nil
	}

	// If 'advance' was called, need to grab a new cursor. Otherwise reuse the
	// initial one, since we haven't touched the query.
	if q.advanced {
		var err error
		c.FinishedCursor, err = q.iter.stop()
		if err != nil {
			return err
		}
	} else {
		c.FinishedCursor = q.cursor
	}

	// Store the current head as well, to be able to return it in 'peek' next
	// time. Note that the cursor points to an item AFTER the head, that's the
	// reason we store the head separately.
	if q.head != nil {
		c.NextFinished = q.head.ID
	} else {
		c.NextFinished = 0
	}

	return nil
}

// close releases the resources used by the query.
//
// Must be called from a defer to make sure no goroutines are leaked.
func (q *invDatastoreQuery) close() {
	if q.iter != nil {
		q.iter.stop()
	}
}

func finishedInvQuery(c context.Context, q *datastore.Query, cur invocationsCursor, parent *datastore.Key) *invDatastoreQuery {
	out := &invDatastoreQuery{}
	if cur.NextFinished < 0 {
		out.done = true
		return out
	}

	if cur.FinishedCursor != nil {
		q = q.Start(cur.FinishedCursor)
		out.cursor = cur.FinishedCursor
	}

	if cur.NextFinished != 0 {
		out.head = &Invocation{ID: cur.NextFinished, JobKey: parent}
	}

	out.iter = &invDatastoreIter{}
	out.iter.start(c, q)
	return out
}
