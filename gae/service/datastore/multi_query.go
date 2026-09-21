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
	"context"
	"iter"
	"sort"
	"sync"

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/data/stringset"
)

type subqueryState struct {
	idx              int
	pull             func() (PropertyMap, error, bool)
	stop             func()
	rawCursor        RawCursorCB
	rawCurrentCursor RawCursorCB
	startBound       RawCursor
	orders           []IndexColumn

	hasPulled      bool
	exhausted      bool
	terminalCursor RawCursor

	pm       PropertyMap
	key      *Key
	keyStr   string
	orderStr string
}

func newSubqueryState(ctx context.Context, idx int, fq *FinalizedQuery) *subqueryState {
	start, _ := fq.Bounds()
	subIter := Raw(ctx).RunQuery(fq)
	pull, stop := iter.Pull2(subIter.Results)
	return &subqueryState{
		idx:              idx,
		pull:             pull,
		stop:             stop,
		rawCursor:        subIter.Cursor,
		rawCurrentCursor: subIter.CurrentCursor,
		startBound:       start,
		orders:           fq.Orders(),
	}
}

func (s *subqueryState) next() (bool, error) {
	pm, err, ok := s.pull()
	if err != nil {
		return false, err
	}
	if !ok {
		s.exhausted = true
		s.pm = nil
		s.key = nil
		s.keyStr = ""
		s.orderStr = ""
		if s.rawCursor != nil {
			s.terminalCursor, _ = s.rawCursor()
		}
		return false, nil
	}

	s.hasPulled = true
	s.pm = pm
	s.key = mustGetKeyFromPM(pm)
	s.keyStr = string(Serialize.ToBytes(s.key))
	s.orderStr = computeOrderStr(s.orders, pm, s.key)
	return true, nil
}

func (s *subqueryState) cursor() (RawCursor, error) {
	if s.exhausted {
		return s.terminalCursor, nil
	}
	if !s.hasPulled {
		return s.startBound, nil
	}
	return s.rawCursor()
}

func (s *subqueryState) currentCursor() (RawCursor, error) {
	if s.exhausted {
		return s.terminalCursor, nil
	}
	if !s.hasPulled {
		return s.startBound, nil
	}
	return s.rawCurrentCursor()
}

func (s *subqueryState) close() {
	if s.stop != nil {
		s.stop()
	}
}

type subqueryHeap []*subqueryState

var _ heap.Interface = &subqueryHeap{}

func (h subqueryHeap) Len() int { return len(h) }

func (h subqueryHeap) Less(i, j int) bool {
	if h[i].orderStr != h[j].orderStr {
		return h[i].orderStr < h[j].orderStr
	}
	return h[i].idx < h[j].idx
}

func (h subqueryHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *subqueryHeap) Push(x any) {
	*h = append(*h, x.(*subqueryState))
}

func (h *subqueryHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

func computeOrderStr(order []IndexColumn, pm PropertyMap, key *Key) string {
	invBuf := cmpbin.Invertible(&bytes.Buffer{})
	for _, column := range order {
		invBuf.SetInvert(column.Descending)
		if column.Property == "__key__" {
			panicIf(Serialize.Key(invBuf, key))
			continue
		}
		columnData := pm[column.Property].Slice()
		sort.Sort(columnData)
		if column.Descending {
			panicIf(Serialize.Property(invBuf, columnData[columnData.Len()-1]))
		} else {
			panicIf(Serialize.Property(invBuf, columnData[0]))
		}
	}
	return invBuf.String()
}

func runMultiRawQuery(ctx context.Context, finalized []*FinalizedQuery) RawQueryIter {
	states := make([]*subqueryState, len(finalized))
	for i, fq := range finalized {
		states[i] = newSubqueryState(ctx, i, fq)
	}

	var mu sync.Mutex
	yieldedSubquery := -1
	yieldedAny := false

	cursorCB := func() (RawCursor, error) {
		mu.Lock()
		defer mu.Unlock()
		if !yieldedAny {
			ret := make(Cursor, len(states))
			for i, s := range states {
				ret[i] = s.startBound
			}
			return ret, nil
		}
		ret := make(Cursor, len(states))
		for i, s := range states {
			var cur RawCursor
			var err error
			if i == yieldedSubquery {
				cur, err = s.cursor()
			} else {
				cur, err = s.currentCursor()
			}
			if err != nil {
				return nil, err
			}
			ret[i] = cur
		}
		return ret, nil
	}

	currentCursorCB := func() (RawCursor, error) {
		mu.Lock()
		defer mu.Unlock()
		if !yieldedAny {
			return nil, ErrNoCurrentCursor
		}
		ret := make(Cursor, len(states))
		for i, s := range states {
			cur, err := s.currentCursor()
			if err != nil {
				return nil, err
			}
			ret[i] = cur
		}
		return ret, nil
	}

	results := func(yield func(PropertyMap, error) bool) {
		defer func() {
			for _, s := range states {
				s.close()
			}
		}()

		h := make(subqueryHeap, 0, len(states))
		for _, s := range states {
			ok, err := s.next()
			if err != nil {
				yield(nil, err)
				return
			}
			if ok {
				h = append(h, s)
			}
		}
		heap.Init(&h)

		orders := finalized[0].Orders()
		isKeyOnly := len(orders) == 1 && orders[0].Property == "__key__"

		var seenKey func(keyStr string) bool
		if isKeyOnly {
			lastSeen := ""
			seenKey = func(keyStr string) bool {
				if lastSeen == keyStr {
					return true
				}
				lastSeen = keyStr
				return false
			}
		} else {
			seenKeys := stringset.New(128)
			seenKey = func(keyStr string) bool {
				return !seenKeys.Add(keyStr)
			}
		}

		for len(h) > 0 {
			top := h[0]

			if !seenKey(top.keyStr) {
				mu.Lock()
				yieldedSubquery = top.idx
				yieldedAny = true
				mu.Unlock()

				pmYield := top.pm
				if pmYield == nil {
					pmYield = make(PropertyMap, 1)
				}
				pmYield.SetMeta("key", top.key)

				if !yield(pmYield, nil) {
					return
				}
			}

			ok, err := top.next()
			if err != nil {
				yield(nil, err)
				return
			}
			if !ok {
				heap.Pop(&h)
			} else {
				heap.Fix(&h, 0)
			}
		}
	}

	return RawQueryIter{
		Cursor:        cursorCB,
		CurrentCursor: currentCursorCB,
		Results:       results,
	}
}
