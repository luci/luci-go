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

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/errors"
)

// queryIterator is an iterator for datastore query results.
type queryIterator struct {
	order                 []IndexColumn
	currentQueryResult    *rawQueryResult
	itemCh                chan *rawQueryResult
	currentItemOrderCache string          // lazy loading (loaded when `CurrentItemOrder()` is call).
	finalizedQuery        *FinalizedQuery // For use in determining which cursor callback is being called
}

// startQueryIterator starts to run the given query and return the iterator for query results.
func startQueryIterator(ctx context.Context, fq *FinalizedQuery) *queryIterator {
	qi := &queryIterator{
		order:          fq.Orders(),
		itemCh:         make(chan *rawQueryResult),
		finalizedQuery: fq, // Save query for later
	}

	go func() {
		defer close(qi.itemCh)
		err := Raw(ctx).Run(fq, func(k *Key, pm PropertyMap, cursorCB CursorCB) error {
			select {
			case <-ctx.Done():
				return Stop
			case qi.itemCh <- &rawQueryResult{
				key:      k,
				data:     pm,
				cursorCB: cursorCB,
			}:
				return nil
			}
		})
		if err == nil || err == Stop || errors.Contains(err, context.Canceled) || errors.Contains(err, context.DeadlineExceeded) {
			return
		}
		qi.itemCh <- &rawQueryResult{
			err: err,
		}
	}()

	return qi
}

// CurrentItem returns the current query result.
func (qi *queryIterator) CurrentItem() (*Key, PropertyMap, CursorCB) {
	if qi.currentQueryResult == nil {
		return nil, PropertyMap{}, nil
	}
	return qi.currentQueryResult.key, qi.currentQueryResult.data, qi.currentQueryResult.cursorCB
}

// CurrentItemKey returns a serialized current item key.
func (qi *queryIterator) CurrentItemKey() string {
	if qi.currentQueryResult == nil || qi.currentQueryResult.key == nil {
		return ""
	}
	return string(Serialize.ToBytes(qi.currentQueryResult.key))
}

// Query returns the query that produced this iterator
func (qi *queryIterator) FinalizedQuery() *FinalizedQuery {
	return qi.finalizedQuery
}

// CurrentItemOrder returns serialized propertied which fields are used in sorting orders.
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

// Next iterate the next item and put it into currentQueryResult.
// Note: call Next() before calling to any CurrentItemXXX functions to get the right results.
func (qi *queryIterator) Next() error {
	if qi.itemCh == nil {
		panic("item channel for queryIterator is not properly initiated")
	}
	var ok bool
	qi.currentQueryResult, ok = <-qi.itemCh
	qi.currentItemOrderCache = ""
	if !ok {
		return Stop
	}
	return qi.currentQueryResult.err
}

// rawQueryResult captures the result from raw datastore query snapshot.
type rawQueryResult struct {
	key      *Key
	data     PropertyMap
	err      error
	cursorCB CursorCB
}
