// Copyright 2026 The LUCI Authors.
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
	"container/heap"
	"fmt"
	"iter"
	"slices"
	"sync"

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/data/stringset"
)

// rawQueryPullIter is a wrapper around the iter.Pull2 version of a
// RawQueryIter.
//
// In particular it is stateful, retaining the current item.
type rawQueryPullIter struct {
	orders []IndexColumn
	it     RawQueryIter

	puller func() (PropertyMap, error, bool)
	stop   func()

	curItem        PropertyMap
	curKey         *Key
	curItemSortKey string
	err            error
	done           bool
}

// advance loads the next result into the cur* fields.
//
// If the iterator is already done, has no effect.
// If the iterator finishes, this will set done to true.
func (r *rawQueryPullIter) advance() {
	if r.done {
		return
	}
	r.curKey = nil
	r.curItemSortKey = ""

	var ok bool
	r.curItem, r.err, ok = r.puller()
	if !ok || r.err != nil {
		r.stop()
		r.done = true
		return
	}
	r.curKey = mustGetKeyFromPM(r.curItem)

	// Compute curItemSortKey.
	//
	// This is a `cmpbin` []byte slice which is a concatenation of the value of
	// each order property. Note that properties in a PropertyMap are in fact
	// repeated - when the column is sorted ascending, we pick the lowest value.
	// If the slice is sorted descending, we pick the highest value.
	//
	// Finally, if the column is descending, it's binary representation needs to
	// be inverted (this is what `SetInvert` does).
	invBuf := cmpbin.Invertible(&bytes.Buffer{})
	for _, column := range r.orders {
		invBuf.SetInvert(column.Descending)
		if column.Property == "__key__" {
			panicIf(Serialize.Key(invBuf, r.curKey))
			continue
		}
		var value Property
		var ok bool
		if column.Descending {
			value, ok = r.curItem[column.Property].Largest(true)
		} else {
			value, ok = r.curItem[column.Property].Smallest(true)
		}
		if !ok {
			panic("impossible: query result has no indexed data in orderby column")
		}
		panicIf(Serialize.Property(invBuf, value))
	}
	r.curItemSortKey = invBuf.String()
}

// currentCursor returns the cursor for the 'current' item.
//
// If this iterator is done, the 'current' item is the one which WOULD be
// yielded next, in the event that more data is added to the datastore (and,
// thus, is equivalent to Cursor()).
func (r *rawQueryPullIter) currentCursor() (RawCursor, error) {
	if r.done {
		return r.it.Cursor()
	}
	return r.it.CurrentCursor()
}

// rawQueryPullIterHeap is a heap of rawQueryPullIters.
//
// These are kept sorted by the 'curItemSortKey', which is a cmpbin-encoded
// serialization of the "sort order" suffix of the queries in the mulit-query.
type rawQueryPullIterHeap []*rawQueryPullIter

var _ heap.Interface = &rawQueryPullIterHeap{}

func (h rawQueryPullIterHeap) Len() int { return len(h) }

func (h rawQueryPullIterHeap) Less(i, j int) bool {
	return h[i].curItemSortKey < h[j].curItemSortKey
}

func (h rawQueryPullIterHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *rawQueryPullIterHeap) Push(x any) {
	*h = append(*h, x.(*rawQueryPullIter))
}

func (h *rawQueryPullIterHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

type multiQueryState struct {
	live rawQueryPullIterHeap

	// seenKey returns true if *Key has been seen by `seenKey` before.
	seenKey func(*Key) bool
}

func (m *multiQueryState) initialize() {
	for _, it := range m.live {
		it.puller, it.stop = iter.Pull2(it.it.Results)
	}

	var wg sync.WaitGroup
	for _, it := range m.live {
		wg.Go(it.advance)
	}
	wg.Wait()
	m.live = slices.DeleteFunc(m.live, func(it *rawQueryPullIter) bool {
		// We remove it if starting the query immediately exhausted it without
		// error. If it's got an error, then leave it in to yield to the caller.
		return it.done && it.err == nil
	})
	if len(m.live) > 0 {
		heap.Init(&m.live)
		if top := m.top(); top.err == nil {
			m.seenKey(top.curKey)
		}
	}
}

// Gets the next-available pull iter (or nil if there are no more live
// iterators)
func (m *multiQueryState) top() *rawQueryPullIter {
	if len(m.live) == 0 {
		return nil
	}
	return m.live[0]
}

// advance will pull on iterators in the heap and re-order them until:
//   - the top iterator has a never-before-seen key
//   - the top iterator has an error
//   - the heap is empty (all subqueries are completed)
func (m *multiQueryState) advance() {
	for {
		top := m.top()
		if top == nil {
			return
		}
		top.advance()
		if top.err != nil {
			return
		}

		if top.done {
			heap.Pop(&m.live)
			if len(m.live) == 0 {
				return
			}
		} else {
			heap.Fix(&m.live, 0)
		}
		top = m.top()

		if !m.seenKey(top.curKey) {
			return
		}
	}
}

func (m *multiQueryState) stop() {
	for _, it := range m.live {
		it.stop()
	}
}

// makeMultiQuerySeenKeys returns a stateful function suitable for tracking
// seen keys in a multi query.
func makeMultiQuerySeenKeys(finalized []*FinalizedQuery) func(k *Key) bool {
	if fq := finalized[0]; len(fq.orders) == 1 && fq.orders[0].Property == "__key__" {
		// if our queries are ordered exclusively by key, then we can implement
		// a very simple 'seen keys' algorithm.
		var lastKey *Key
		return func(k *Key) bool {
			if k.Equal(lastKey) {
				return true
			}
			lastKey = k
			return false
		}
	}

	// Otherwise we need to use a set.
	seenKeys := stringset.New(128)
	return func(key *Key) bool {
		return !seenKeys.Add(string(Serialize.ToBytes(key)))
	}
}

func sortAndCompactQueries(queries []*Query) []*Query {
	sortedQueries := slices.Clone(queries)
	slices.SortStableFunc(sortedQueries, (*Query).Compare)
	return slices.CompactFunc(sortedQueries, func(a, b *Query) bool {
		return a.Compare(b) == 0
	})
}

// prepareQueries keys-only's the queries, sorts and finalizes them.
func prepareQueries(isKey bool, queries []*Query) ([]*FinalizedQuery, error) {
	sortedQueries := sortAndCompactQueries(queries)

	finalized := make([]*FinalizedQuery, len(sortedQueries))
	overallKind := ""
	overallOrder := ""
	var proj []string
	for i, sq := range sortedQueries {
		if isKey && sq.project.Len() == 0 {
			sq = sq.KeysOnly(true)
		}
		fq, err := sq.Finalize()
		if err != nil {
			return nil, err
		}
		// If we have more than one query (multi-query), AND this query is not
		// loading the full entity (e.g. keys-only or projection) AND we have more
		// than one sort order, we need to project all the sort order columns into
		// our result in order to be able to correctly order the queries in the
		// heap.
		if len(sortedQueries) > 1 && (fq.KeysOnly() || len(fq.project) > 0) && len(fq.orders) > 1 {
			if proj == nil {
				proj = make([]string, 0, len(fq.orders)-1)
				for _, col := range fq.orders {
					if col.Property != "__key__" {
						proj = append(proj, col.Property)
					}
				}
			}
			fq, err = sq.KeysOnly(false).Project(proj...).Finalize()
			if err != nil {
				return nil, err
			}
		}
		finalized[i] = fq

		order := ""
		for j, col := range fq.orders {
			if j != 0 {
				order += ","
			}
			order += col.String()
		}
		switch {
		case i == 0:
			overallKind = fq.kind
			overallOrder = order
		case fq.kind != overallKind:
			return nil, fmt.Errorf(
				"all RunQuery queries should query the same kind, but got %q and %q",
				fq.kind, overallKind)
		case order != overallOrder:
			return nil, fmt.Errorf(
				"all RunQuery queries should use the same order, but got %q and %q",
				order, overallOrder)
		}
	}

	return finalized, nil
}
