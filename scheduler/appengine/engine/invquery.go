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
	"encoding/base64"
	"sort"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/scheduler/appengine/internal"
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
// It picks smallest IDs first. In presence of duplicates, it favors queries
// that are listed in 'qs' earlier.
//
// Places the results into 'out' slice, returning the extended slice at the end.
//
// Returns (..., true, nil) in case results of all queries has been exhausted,
// and (..., false, nil) if it hit the limit on number of results.
func mergeInvQueries(qs []invQuery, limit int, out []*Invocation) ([]*Invocation, bool, error) {
	maxLen := len(out) + limit

	for len(out) != maxLen {
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
			return out, true, nil // exhausted results of all queries
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

	return out, false, nil // fetched up to the limit
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
// Smallest IDs are returned first. IDs smaller or equal than lastScanned are
// skipped (this is used for pagination).
func activeInvQuery(c context.Context, j *Job, lastScanned int64) *invListQuery {
	var invs []*Invocation
	for _, id := range j.ActiveInvocations {
		if id > lastScanned {
			invs = append(invs, &Invocation{ID: id})
		}
	}
	sort.Slice(invs, func(l, r int) bool { return invs[l].ID < invs[r].ID })
	return &invListQuery{invs: invs}
}

// recentInvQuery returns invQuery that emits recently finished invocations, as
// fetched from the job.FinishedInvocationsRaw field.
//
// Smallest IDs are returned first. IDs smaller or equal than lastScanned are
// skipped (this is used for pagination).
func recentInvQuery(c context.Context, j *Job, lastScanned int64) *invListQuery {
	finished, err := filteredFinishedInvs(
		j.FinishedInvocationsRaw, clock.Now(c).Add(-FinishedInvocationsHorizon))
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to unmarshal FinishedInvocationsRaw, skipping")
		return &invListQuery{}
	}

	var invs []*Invocation
	for _, inv := range finished {
		if inv.InvocationId > lastScanned {
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

// invDatastoreQuery implements invQuery on top of a datastore iterator.
//
// The datastore query results are expected to be sorted by IDs in
// smallest-to-largest order. This is also the order in which invocations will
// be returned.
type invDatastoreQuery struct {
	iter invDatastoreIter // iterator positioned before the next result
	head *Invocation      // value to return in peek() or nil if haven't fetched yet
	err  error            // non-nil if the last fetch failed
	done bool             // true if fetched everything we could
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
	q.head, q.err = q.iter.next()
	q.done = q.head == nil && q.err == nil
	return q.err
}

// close releases the resources used by the query.
//
// Must be called from a defer to make sure no goroutines are leaked.
func (q *invDatastoreQuery) close() {
	q.iter.stop()
}

// finishedInvQuery returns invQuery that emits historical finished invocations,
// as fetched from the given datastore query.
//
// Smallest IDs are returned first. IDs smaller or equal than lastScanned are
// skipped (this is used for pagination).
func finishedInvQuery(c context.Context, q *datastore.Query, parent *datastore.Key, lastScanned int64) *invDatastoreQuery {
	if lastScanned > 0 {
		q = q.Gt("__key__", datastore.KeyForObj(c, &Invocation{
			ID:     lastScanned,
			JobKey: parent,
		}))
	}
	q = q.Order("__key__")
	out := &invDatastoreQuery{}
	out.iter.start(c, q)
	return out
}

////////////////////////////////////////////////////////////////////////////////
// Cursor helpers.

// decodeInvCursor deserializes a base64-encoded cursor.
func decodeInvCursor(cursor string, cur *internal.InvocationsCursor) error {
	if cursor == "" {
		*cur = internal.InvocationsCursor{}
		return nil
	}

	blob, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return errors.Annotate(err, "failed to base64 decode the cursor").Err()
	}

	if err = proto.Unmarshal(blob, cur); err != nil {
		return errors.Annotate(err, "failed to unmarshal the cursor").Err()
	}

	return nil
}

// encodeInvCursor serializes the cursor to base64-encoded string.
func encodeInvCursor(cur *internal.InvocationsCursor) (string, error) {
	zero := internal.InvocationsCursor{}
	if *cur == zero {
		return "", nil
	}

	blob, err := proto.Marshal(cur)
	if err != nil {
		return "", err // must never actually happen
	}

	return base64.RawURLEncoding.EncodeToString(blob), nil
}

////////////////////////////////////////////////////////////////////////////////
// High level functions used by Engine.

// invsPage contains information about a page returned by fetchInvsPage.
type invsPage struct {
	count       int   // number of invocations in the page
	final       bool  // true if this is the final page
	lastScanned int64 // ID of the last scanned invocation
}

// fetchInvsPage fetches (perhaps incomplete or empty) page of invocations,
// adding them to the given 'out' slice.
//
// Uses 'lastScanned' as a resumption cursor: only invocations with IDs larger
// than 'lastScanned' will be fetched.
//
// It is called (perhaps multiple times) by public ListInvocations to construct
// a full page of results out of smaller incomplete pages.
//
// Returns the extended 'out' slice (that now contains fetched items) and
// information about the fetched page.
func fetchInvsPage(c context.Context, job *Job, opts ListInvocationsOpts, lastScanned int64, out []*Invocation) ([]*Invocation, invsPage, error) {
	// We are going to merge results of multiple queries:
	//   1) Over historical finished invocations in the datastore.
	//   2) Over recently finished invocations, stored inline in the Job entity.
	//   3) Over active invocations, also stored inline in the Job entity.
	var qs []invQuery

	// Prepare the queries over finished invocations.
	if !opts.ActiveOnly {
		var parent *datastore.Key
		q := datastore.NewQuery("Invocation")
		if job.IsV2() {
			q = q.Eq("IndexedJobID", job.JobID)
		} else {
			parent = datastore.NewKey(c, "Job", job.JobID, 0, nil)
			q = q.Ancestor(parent)
		}

		finishedQuery := finishedInvQuery(c, q, parent, lastScanned)
		defer finishedQuery.close()
		qs = append(qs, finishedQuery)

		// And always grab recently finished invocations from the Job, since they
		// may be more up-to-date.
		qs = append(qs, recentInvQuery(c, job, lastScanned))
	}

	// Prepare the query over active invocations.
	if !opts.FinishedOnly {
		qs = append(qs, activeInvQuery(c, job, lastScanned))
	}

	// Merge the results of all queries.
	prevSize := len(out)
	out, final, err := mergeInvQueries(qs, opts.PageSize, out)
	if err != nil {
		return nil, invsPage{}, errors.Annotate(err, "failed to query invocations").Tag(transient.Tag).Err()
	}

	// Nothing new at all? We are done. Otherwise remember the last ID we looked
	// at to resume our query from it. It is important to grab the ID before
	// the filtering, otherwise we may end up stuck in an infinite loop that
	// fetches an empty page (with all items filtered out) over and over again,
	// not advancing the query.
	if len(out) == prevSize {
		return out, invsPage{0, true, lastScanned}, nil
	}
	lastScanned = out[len(out)-1].ID

	// Inflate and filter (in-place) shallow entities resulted from queries over
	// IDs list. Note that this may reduce the returned page size, in
	// a pathological case to 0. 'ListInvocations' will compensate for that by
	// calling 'fetchInvsPage' again to fetch more stuff until the full page is
	// fetched.
	filtered, err := fillShallowInvs(c, out[prevSize:], opts)
	if err != nil {
		return nil, invsPage{}, err
	}

	// 'filtered' points to a subslice of 'out' (located at the end), that has
	// all filtered items now. Truncate 'out' to get rid of garbage left after
	// the filtering.
	out = out[:prevSize+len(filtered)]

	return out, invsPage{len(out) - prevSize, final, lastScanned}, nil
}

// fillShallowInvs detects entities that do not have bodies fetched yet, fetches
// them, and filters them based on ActiveOnly/FinishedOnly filter defined by
// opts.
//
// This is needed for results of queries that use IDs inlined in the Job entity.
// We detect such shallow entities by missing Status value, which is guaranteed
// to be set for all Invocation entities.
//
// Filtering is required since the state of the entities fetched here may be
// more up-to-date than the state used by queries. In particular, active
// invocations may not be active anymore.
//
// Filters the given slice in-place and returns the filtered slice that shares
// same underlying array.
func fillShallowInvs(c context.Context, invs []*Invocation, opts ListInvocationsOpts) ([]*Invocation, error) {
	var shallow []*Invocation
	for _, inv := range invs {
		if inv.Status == "" {
			shallow = append(shallow, inv)
		}
	}
	if len(shallow) == 0 {
		return invs, nil
	}

	if err := datastore.Get(c, shallow); err != nil {
		return nil, errors.Annotate(err, "failed to fetch invocations").Tag(transient.Tag).Err()
	}

	filtered := invs[:0]
	for _, inv := range invs {
		if opts.ActiveOnly && inv.Status.Final() {
			continue
		}
		if opts.FinishedOnly && !inv.Status.Final() {
			continue
		}
		filtered = append(filtered, inv)
	}
	return filtered, nil
}
