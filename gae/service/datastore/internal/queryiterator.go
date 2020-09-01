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
	"errors"
	"sort"

	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/datastore/serialize"
)

// QueryIterator is an iterator for datastore query results.
type QueryIterator struct {
	order []ds.IndexColumn
	currentQueryResult *rawQueryResult
	itemCh chan *rawQueryResult
}

// NewQueryIterator initiate a QueryIterator.
func NewQueryIterator(order []ds.IndexColumn) *QueryIterator{
	return &QueryIterator{
		order:              order,
		itemCh:             make(chan *rawQueryResult),
	}
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
	return string(serialize.ToBytes(qi.currentQueryResult.key))
}

// CurrentItemOrder returns serialized propertied which fields are used in sorting orders.
func (qi *QueryIterator) CurrentItemOrder() (s string, err error) {
	if qi.currentQueryResult == nil {
		return
	}
	invBuf:= serialize.Invertible(&bytes.Buffer{})
	for _, column := range qi.order {
		if column.Property == "__key__" {
			continue
		}
		invBuf.SetInvert(column.Descending)
		columnData := qi.currentQueryResult.data[column.Property].Slice()
		sort.Sort(columnData)
		if column.Descending {
			for i := columnData.Len() - 1; i >= 0; i-- {
				err = serialize.WriteProperty(invBuf, false, columnData[i])
			}
		} else {
			for i := 0; i < columnData.Len(); i ++ {
				err = serialize.WriteProperty(invBuf, false, columnData[i])
			}
		}
	}
	return invBuf.String(), err
}

// Next iterate the next item and put it into currentQueryResult.
// Note: call Next() before calling to any CurrentItemXXX functions to get the right results.
func (qi *QueryIterator) Next() error {
	if qi.itemCh == nil {
		return errors.New("item channel for QueryIterator is not properly initiated")
	}
	var ok bool
	qi.currentQueryResult, ok = <- qi.itemCh
	if !ok {
		return ds.Stop
	}
	return nil
}

// rawQueryResult captures the result from raw datastore query snapshot.
type rawQueryResult struct {
	key *ds.Key
	data ds.PropertyMap
}
