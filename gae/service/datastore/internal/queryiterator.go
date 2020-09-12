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
	ds "go.chromium.org/luci/gae/service/datastore"
)

// QueryIterator is an iterator for datastore query results.
type QueryIterator struct {
	order              []ds.IndexColumn
	currentQueryResult *rawQueryResult
	itemCh             chan *rawQueryResult
}

// StartQueryIterator starts to run the given query and return the iterator for query results.
func StartQueryIterator(ctx context.Context, fq *ds.FinalizedQuery) *QueryIterator {
	qi := &QueryIterator{
		order:  fq.Orders(),
		itemCh: make(chan *rawQueryResult),
	}

	go func(fq *ds.FinalizedQuery, qi *QueryIterator) {
		defer close(qi.itemCh)
		err := ds.Raw(ctx).Run(fq, func(k *ds.Key, pm ds.PropertyMap, _ ds.CursorCB) error {
			select {
			case <-ctx.Done():
				return ds.Stop
			case qi.itemCh <- &rawQueryResult{
				key:  k,
				data: pm,
			}:
				return nil
			}
		})
		if err != nil {
			qi.itemCh <- &rawQueryResult{
				err: err,
			}
		}
	}(fq, qi)

	return qi
}

// CurrentItem returns the current query result.
func (qi *QueryIterator) CurrentItem() (*ds.Key, ds.PropertyMap) {
	if qi.currentQueryResult == nil {
		return nil, ds.PropertyMap{}
	}
	return qi.currentQueryResult.key, qi.currentQueryResult.data
}

// CurrentItemKey returns a serialized current item key.
func (qi *QueryIterator) CurrentItemKey() string {
	if qi.currentQueryResult == nil || qi.currentQueryResult.key == nil {
		return ""
	}
	return string(ds.Serialize.ToBytes(qi.currentQueryResult.key))
}

// CurrentItemOrder returns serialized propertied which fields are used in sorting orders.
func (qi *QueryIterator) CurrentItemOrder() (s string, err error) {
	if qi.currentQueryResult == nil {
		return
	}

	invBuf := cmpbin.Invertible(&bytes.Buffer{})
	for _, column := range qi.order {
		invBuf.SetInvert(column.Descending)
		if column.Property == "__key__" {
			if err = ds.Serialize.Key(invBuf, qi.currentQueryResult.key); err != nil {
				return
			}
			continue
		}
		columnData := qi.currentQueryResult.data[column.Property].Slice()
		sort.Sort(columnData)
		if column.Descending {
			err = ds.Serialize.Property(invBuf, columnData[columnData.Len()-1])
		} else {
			err = ds.Serialize.Property(invBuf, columnData[0])
		}
		if err != nil {
			return
		}
	}
	return invBuf.String(), err
}

// Next iterate the next item and put it into currentQueryResult.
// Note: call Next() before calling to any CurrentItemXXX functions to get the right results.
func (qi *QueryIterator) Next() error {
	if qi.itemCh == nil {
		panic("item channel for QueryIterator is not properly initiated")
	}
	var ok bool
	qi.currentQueryResult, ok = <-qi.itemCh
	if !ok {
		return ds.Stop
	}
	return qi.currentQueryResult.err
}

// rawQueryResult captures the result from raw datastore query snapshot.
type rawQueryResult struct {
	key  *ds.Key
	data ds.PropertyMap
	err  error
}
