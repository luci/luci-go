// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package bigtable

import (
	"bytes"
	"fmt"
	"time"

	"github.com/luci/gkvlite"

	"github.com/luci/luci-go/common/data/recordio"
	"github.com/luci/luci-go/logdog/common/storage"

	"golang.org/x/net/context"
)

// btTableTest is an in-memory implementation of btTable interface for testing.
//
// This is a simple implementation; not an efficient one.
type btTableTest struct {
	s *gkvlite.Store
	c *gkvlite.Collection

	// err, if true, is the error immediately returned by functions.
	err error

	// maxLogAge is the currently-configured maximum log age.
	maxLogAge time.Duration
}

// Testing is an extension of storage.Storage with additional testing
// capabilities.
type Testing interface {
	storage.Storage

	DataMap() map[string][]byte
	SetMaxRowSize(int)
	SetErr(error)
	MaxLogAge() time.Duration
}

type btTestingStorage struct {
	*btStorage
	mem *btTableTest
}

func (st *btTestingStorage) Close() {
	// Override Close to make sure our gkvlite instance is closed.
	st.mem.close()
	st.btStorage.Close()
}

func (st *btTestingStorage) DataMap() map[string][]byte { return st.mem.dataMap() }
func (st *btTestingStorage) SetMaxRowSize(v int)        { st.maxRowSize = v }
func (st *btTestingStorage) SetErr(err error)           { st.mem.err = err }
func (st *btTestingStorage) MaxLogAge() time.Duration   { return st.mem.maxLogAge }

// NewMemoryInstance returns an in-memory BigTable Storage implementation.
// This can be supplied in the Raw field in Options to simulate a BigTable
// connection.
//
// Close should be called on the resulting value after the user is finished in
// order to free resources.
func NewMemoryInstance(c context.Context, opts Options) Testing {
	mem := &btTableTest{}
	base := newBTStorage(c, opts, nil, nil, mem)
	return &btTestingStorage{
		btStorage: base,
		mem:       mem,
	}
}

func (t *btTableTest) close() {
	if t.s != nil {
		t.s.Close()
		t.s = nil
	}
}

func (t *btTableTest) collection() *gkvlite.Collection {
	if t.s == nil {
		var err error
		t.s, err = gkvlite.NewStore(nil)
		if err != nil {
			panic(err)
		}
		t.c = t.s.MakePrivateCollection(bytes.Compare)
	}
	return t.c
}

func (t *btTableTest) putLogData(c context.Context, rk *rowKey, d []byte) error {
	if t.err != nil {
		return t.err
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
	coll := t.collection()
	if item, _ := coll.Get(enc); item != nil {
		return storage.ErrExists
	}

	clone := make([]byte, len(d))
	copy(clone, d)
	if err := coll.Set(enc, clone); err != nil {
		panic(err)
	}
	return nil
}

func (t *btTableTest) getLogData(c context.Context, rk *rowKey, limit int, keysOnly bool, cb btGetCallback) error {
	if t.err != nil {
		return t.err
	}

	enc := []byte(rk.encode())
	prefix := rk.pathPrefix()
	var ierr error
	err := t.collection().VisitItemsAscend(enc, !keysOnly, func(i *gkvlite.Item) bool {
		var drk *rowKey
		drk, ierr = decodeRowKey(string(i.Key))
		if ierr != nil {
			return false
		}
		if drk.pathPrefix() != prefix {
			return false
		}

		rowData := i.Val
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
	if err != nil {
		panic(err)
	}
	return ierr
}

func (t *btTableTest) setMaxLogAge(c context.Context, d time.Duration) error {
	if t.err != nil {
		return t.err
	}
	t.maxLogAge = d
	return nil
}

func (t *btTableTest) dataMap() map[string][]byte {
	result := map[string][]byte{}

	err := t.collection().VisitItemsAscend([]byte(nil), true, func(i *gkvlite.Item) bool {
		result[string(i.Key)] = i.Val
		return true
	})
	if err != nil {
		panic(err)
	}
	return result
}
