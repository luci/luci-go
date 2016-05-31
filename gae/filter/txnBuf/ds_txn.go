// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package txnBuf

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
)

// ErrTransactionTooLarge is returned when applying an inner transaction would
// cause an outer transaction to become too large.
var ErrTransactionTooLarge = errors.New(
	"applying the transaction would make the parent transaction too large")

// ErrTooManyRoots is returned when executing an operation which would cause
// the transaction to exceed it's allotted number of entity groups.
var ErrTooManyRoots = errors.New(
	"operating on too many entity groups in nested transaction")

type dsTxnBuf struct {
	ic       context.Context
	state    *txnBufState
	haveLock bool
}

var _ ds.RawInterface = (*dsTxnBuf)(nil)

func (d *dsTxnBuf) DecodeCursor(s string) (ds.Cursor, error) {
	return d.state.parentDS.DecodeCursor(s)
}

func (d *dsTxnBuf) AllocateIDs(incomplete *ds.Key, n int) (start int64, err error) {
	return d.state.parentDS.AllocateIDs(incomplete, n)
}

func (d *dsTxnBuf) GetMulti(keys []*ds.Key, metas ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	return d.state.getMulti(keys, metas, cb, d.haveLock)
}

func (d *dsTxnBuf) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) error {
	return d.state.putMulti(keys, vals, cb, d.haveLock)
}

func (d *dsTxnBuf) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	return d.state.deleteMulti(keys, cb, d.haveLock)
}

func (d *dsTxnBuf) Count(fq *ds.FinalizedQuery) (count int64, err error) {
	// Unfortunately there's no fast-path here. We literally have to run the
	// query and count. Fortunately we can optimize to count keys if it's not
	// a projection query. This will save on bandwidth a bit.
	if len(fq.Project()) == 0 && !fq.KeysOnly() {
		fq, err = fq.Original().KeysOnly(true).Finalize()
		if err != nil {
			return
		}
	}
	err = d.Run(fq, func(_ *ds.Key, _ ds.PropertyMap, _ ds.CursorCB) error {
		count++
		return nil
	})
	return
}

func (d *dsTxnBuf) Run(fq *ds.FinalizedQuery, cb ds.RawRunCB) error {
	if start, end := fq.Bounds(); start != nil || end != nil {
		return errors.New("txnBuf filter does not support query cursors")
	}

	limit, limitSet := fq.Limit()
	offset, _ := fq.Offset()
	keysOnly := fq.KeysOnly()

	project := fq.Project()

	bufDS, parentDS, sizes := func() (ds.RawInterface, ds.RawInterface, *sizeTracker) {
		if !d.haveLock {
			d.state.Lock()
			defer d.state.Unlock()
		}
		return d.state.bufDS, d.state.parentDS, d.state.entState.dup()
	}()

	return runMergedQueries(fq, sizes, bufDS, parentDS, func(key *ds.Key, data ds.PropertyMap) error {
		if offset > 0 {
			offset--
			return nil
		}
		if limitSet {
			if limit == 0 {
				return ds.Stop
			}
			limit--
		}
		if keysOnly {
			data = nil
		} else if len(project) > 0 {
			newData := make(ds.PropertyMap, len(project))
			for _, p := range project {
				newData[p] = data[p]
			}
			data = newData
		}
		return cb(key, data, nil)
	})
}

func (d *dsTxnBuf) RunInTransaction(cb func(context.Context) error, opts *ds.TransactionOptions) error {
	if !d.haveLock {
		d.state.Lock()
		defer d.state.Unlock()
	}
	return withTxnBuf(d.ic, cb, opts)
}

func (d *dsTxnBuf) Testable() ds.Testable {
	return d.state.parentDS.Testable()
}
