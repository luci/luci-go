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

package datastore

import (
	"bytes"
	"context"
	"sort"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/data/cmpbin"
)

// queryIterator is an iterator for datastore query results.
//
// DANGER: This implementation assumes instances of CursorCB carry complete
// snapshots of the query iteration state, e.g. "remembering" a cursor callback
// at some iteration cycle `X`, and calling it a later cycle `Y` will produce
// a cursor for the cycle `X`. This property is not guaranteed by RawInterface
// API and it (accidentally) only holds for `impl/memory` implementation using
// only in unit tests.
//
// In all production implementations cursor callbacks just hold a pointer to
// the internal datastore iterator: calling such callback always returns
// the **current position** of the internal iterator (in the above example, it
// will produce the position representing `Y`, not `X`). As a consequence,
// RunMutli skips entities when resuming from a cursor.
//
// Additionally, RawInterface doesn't guarantee it is safe to call the cursor
// callback from another goroutine (which queryIterator does). This results in
// data races when using cursors with production implementations. Again,
// accidentally, `impl/memory` works "fine", so this problem is obscured in
// unit tests.
//
// One potential fix is to call the cursor callback for every fetched entity and
// pass around real Cursor objects (that are complete "snapshots") instead of
// CursorCB functions (that are just pointers to an internal non-thread safe
// iterator with the most recent cursor). The problem is that CursorCB is a
// potentially slow operations that can make RPCs (and it does in `impl/prod`
// implementation). That's the reason it is a callback (to be called lazily only
// when needed). It is not supposed to be used on every loop cycle. Using it
// this way may severely degrade performance of the query.
type queryIterator struct {
	query                 *Query
	order                 []IndexColumn
	currentQueryResult    *rawQueryResult
	itemCh                chan *rawQueryResult
	done                  bool
	currentItemOrderCache string   // lazy loading (loaded when `CurrentItemOrder()` is called).
	cursorCB              CursorCB // for the *current* item
}

// startQueryIterator starts to run the given query and return the iterator for
// query results.
func startQueryIterator(ctx context.Context, eg *errgroup.Group, fq *FinalizedQuery) *queryIterator {
	qi := &queryIterator{
		query:  fq.Original(),
		order:  fq.Orders(),
		itemCh: make(chan *rawQueryResult),
		// This will be used as CurrentCursor after the first Next() call. To get
		// the first query result, we just need to restart the query from its
		// initial starting cursor.
		cursorCB: func() (Cursor, error) {
			start, _ := fq.Bounds()
			return start, nil
		},
	}

	eg.Go(func() (err error) {
		defer func() { qi.itemCh <- &rawQueryResult{err: err} }()
		return Raw(ctx).Run(fq, func(k *Key, pm PropertyMap, cursorCB CursorCB) error {
			if k == nil { // we use `key == nil` as an indicator of the last message
				panic("impossible per Run contract")
			}
			// Do not even attempt to write to `qi.itemCh` if the context is already
			// done. Note that if multiple cases of select {...} are ready at the same
			// time, Go chooses one randomly to proceed. We don't want that if the
			// context is already done.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case qi.itemCh <- &rawQueryResult{
				key:      k,
				data:     pm,
				cursorCB: cursorCB,
			}:
				return nil
			}
		})
	})

	return qi
}

// Query is the original query this iterator was started with.
func (qi *queryIterator) Query() *Query {
	return qi.query
}

// CurrentItem returns the current query result.
//
// Returns nil key if the iterator has reached its end.
func (qi *queryIterator) CurrentItem() (*Key, PropertyMap) {
	if qi.currentQueryResult == nil {
		return nil, PropertyMap{}
	}
	return qi.currentQueryResult.key, qi.currentQueryResult.data
}

// CurrentItemKey returns a serialized current item key.
//
// Returns "" if the iterator has reached its end.
func (qi *queryIterator) CurrentItemKey() string {
	if qi.currentQueryResult == nil || qi.currentQueryResult.key == nil {
		return ""
	}
	return string(Serialize.ToBytes(qi.currentQueryResult.key))
}

// CurrentCursor returns a cursor pointing to the current item (if any).
//
// The defining property of this cursor is that if a queryIterator is recreated
// with it, its first Next() call will return the current item again (if any).
// This is useful for repopulating the heap when restarting the query from
// a cursor.
//
// Note that if the iterator is exhausted already, i.e. Next() returns
// done == true, CurrentCursor() still returns some non-nil cursor. This cursor
// points to a position right after the last fetched item. When resuming from
// such cursor, we'll either immediately discover the iterator is still
// exhausted, or (if the datastore state changed between calls), we'll discover
// new items that can be fetched now.
//
// Returns nil only if the query produced no results whatsoever and this query
// didn't have a cursor set. In that case we'll need to restart the query from
// scratch when restarting the iteration and this is precisely what `nil` cursor
// does.
func (qi *queryIterator) CurrentCursor() (Cursor, error) {
	return qi.cursorCB()
}

// CurrentItemOrder returns a serialized representation of properties used for
// ordering the results.
//
// Such strings are directly comparable to one another.
func (qi *queryIterator) CurrentItemOrder() string {
	if qi.currentItemOrderCache != "" {
		return qi.currentItemOrderCache
	}

	if qi.currentQueryResult == nil {
		return ""
	}

	invBuf := cmpbin.Invertible(&bytes.Buffer{})
	for _, column := range qi.order {
		invBuf.SetInvert(column.Descending)
		if column.Property == "__key__" {
			panicIf(Serialize.Key(invBuf, qi.currentQueryResult.key))
			continue
		}
		columnData := qi.currentQueryResult.data[column.Property].Slice()
		sort.Sort(columnData)
		if column.Descending {
			panicIf(Serialize.Property(invBuf, columnData[columnData.Len()-1]))
		} else {
			panicIf(Serialize.Property(invBuf, columnData[0]))
		}
	}
	qi.currentItemOrderCache = invBuf.String()
	return qi.currentItemOrderCache
}

// Next iterates to the next item and makes it current.
//
// Note: call Next() before calling any CurrentItemXXX functions to get the
// right results.
//
// If the iterator has finished running returns done == true and an error
// (if the iterator finished due to an error). The error may be a context error
// if the root context was canceled or has expired.
func (qi *queryIterator) Next() (done bool, err error) {
	if qi.itemCh == nil {
		panic("item channel for queryIterator is not properly initiated")
	}
	if !qi.done {
		// Let's assume currentQueryResult index among the full list of query
		// results is `T`. It means `currentQueryResult.cursorCB` is pointing to
		// `T+1`. Also `<-qi.itemCh` will return the next result, i.e. `T+1` as
		// well. This new `T+1` result will become the CurrentItem(). We need to
		// make CurrentCursor() return `T+1` cursor as well. And this is precisely
		// what `currentQueryResult.cursorCB` does.
		//
		// The nil check is for the very first Next() call. We already populated
		// cursorCB correctly for this situation in `startQueryIterator`.
		//
		// See also DANGER warning in queryIterator doc. This is not how cursors
		// actually behave.
		if qi.currentQueryResult != nil {
			qi.cursorCB = qi.currentQueryResult.cursorCB
		}
		qi.currentQueryResult = <-qi.itemCh
		qi.currentItemOrderCache = ""
		qi.done = qi.currentQueryResult.key == nil
	}
	return qi.done, qi.currentQueryResult.err
}

// rawQueryResult captures the result from raw datastore query snapshot.
type rawQueryResult struct {
	key      *Key
	data     PropertyMap
	err      error
	cursorCB CursorCB // points to the entry right after `key`
}
