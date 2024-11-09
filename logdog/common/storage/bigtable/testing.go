// Copyright 2016 The LUCI Authors.
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

package bigtable

import (
	"bytes"
	"context"
	"fmt"

	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/data/treapstore"

	"go.chromium.org/luci/logdog/common/storage"
)

type storageItem struct {
	key   []byte
	value []byte
}

// Testing is an extension of storage.Storage with additional testing
// capabilities.
type Testing interface {
	storage.Storage

	DataMap() map[string][]byte
	SetMaxRowSize(int)
	SetErr(error)
}

type btTestingStorage struct {
	*Storage

	maxRowSize int

	s *treapstore.Store
	c *treapstore.Collection

	// err, if true, is the error immediately returned by functions.
	err error
}

func (bts *btTestingStorage) DataMap() map[string][]byte { return bts.dataMap() }
func (bts *btTestingStorage) SetMaxRowSize(v int)        { bts.maxRowSize = v }
func (bts *btTestingStorage) SetErr(err error)           { bts.err = err }

// NewMemoryInstance returns an in-memory BigTable Storage implementation.
// This can be supplied in the Raw field in Options to simulate a BigTable
// connection.
//
// An optional cache can be supplied to test caching logic.
//
// Close should be called on the resulting value after the user is finished in
// order to free resources.
func NewMemoryInstance(cache storage.Cache) Testing {
	s := Storage{
		LogTable: "test-log-table",
		Cache:    cache,
	}

	ts := treapstore.New()
	bts := btTestingStorage{
		Storage:    &s,
		maxRowSize: bigTableRowMaxBytes,
		s:          ts,
		c: ts.CreateCollection("", func(a, b any) int {
			return bytes.Compare(a.(*storageItem).key, b.(*storageItem).key)
		}),
	}

	// Set our BigTable interface to the in-memory testing instance.
	s.testBTInterface = &bts
	return &bts
}

func (bts *btTestingStorage) putLogData(c context.Context, rk *rowKey, d []byte) error {
	if bts.err != nil {
		return bts.err
	}

	// Record/count sanity check.
	records, err := recordio.Split(d)
	if err != nil {
		return err
	}
	if int64(len(records)) != rk.count {
		return fmt.Errorf("count mismatch (%d != %d)", len(records), rk.count)
	}

	enc := []byte(rk.encode())
	if item := bts.c.Get(&storageItem{enc, nil}); item != nil {
		return storage.ErrExists
	}

	clone := make([]byte, len(d))
	copy(clone, d)
	bts.c.Put(&storageItem{enc, clone})

	return nil
}

func (bts *btTestingStorage) dropRowRange(c context.Context, rk *rowKey) error {
	it := bts.c.Iterator(&storageItem{[]byte(rk.pathPrefix()), nil})
	for {
		itm, ok := it.Next()
		if !ok {
			return nil
		}
		drk, err := decodeRowKey(string((itm.(*storageItem)).key))
		if err != nil {
			return err
		}
		if !drk.sharesPathWith(rk) {
			return nil
		}
		bts.c.Delete(itm)
	}
}

func (bts *btTestingStorage) forEachItem(start []byte, cb func(k, v []byte) bool) {
	it := bts.c.Iterator(&storageItem{start, nil})
	for {
		itm, ok := it.Next()
		if !ok {
			return
		}
		ent := itm.(*storageItem)
		if !cb(ent.key, ent.value) {
			return
		}
	}
}

func (bts *btTestingStorage) getLogData(c context.Context, rk *rowKey, limit int, keysOnly bool, cb btGetCallback) error {
	if bts.err != nil {
		return bts.err
	}

	enc := []byte(rk.encode())
	prefix := rk.pathPrefix()
	var ierr error

	bts.forEachItem(enc, func(k, v []byte) bool {
		var drk *rowKey
		drk, ierr = decodeRowKey(string(k))
		if ierr != nil {
			return false
		}
		if drk.pathPrefix() != prefix {
			return false
		}

		rowData := v
		if keysOnly {
			rowData = nil
		}

		if ierr = cb(drk, rowData); ierr != nil {
			if ierr == errStop {
				ierr = nil
			}
			return false
		}

		if limit > 0 {
			limit--
			if limit == 0 {
				return false
			}
		}

		return true
	})
	return ierr
}

func (bts *btTestingStorage) dataMap() map[string][]byte {
	result := map[string][]byte{}

	bts.forEachItem(nil, func(k, v []byte) bool {
		result[string(k)] = v
		return true
	})
	return result
}

func (bts *btTestingStorage) getMaxRowSize() int { return bts.maxRowSize }
