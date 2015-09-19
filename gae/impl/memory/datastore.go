// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"

	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
)

//////////////////////////////////// public ////////////////////////////////////

// useRDS adds a gae.Datastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return ds.SetRawFactory(c, func(ic context.Context) ds.RawInterface {
		dsd := cur(ic).Get(memContextDSIdx)

		ns := curGID(ic).namespace
		if x, ok := dsd.(*dataStoreData); ok {
			return &dsImpl{x, ns, ic}
		}
		return &txnDsImpl{dsd.(*txnDataStoreData), ns}
	})
}

//////////////////////////////////// dsImpl ////////////////////////////////////

// dsImpl exists solely to bind the current c to the datastore data.
type dsImpl struct {
	data *dataStoreData
	ns   string
	c    context.Context
}

var _ ds.RawInterface = (*dsImpl)(nil)

func (d *dsImpl) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) error {
	d.data.putMulti(keys, vals, cb)
	return nil
}

func (d *dsImpl) GetMulti(keys []*ds.Key, _meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	return d.data.getMulti(keys, cb)
}

func (d *dsImpl) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	d.data.delMulti(keys, cb)
	return nil
}

func (d *dsImpl) DecodeCursor(s string) (ds.Cursor, error) {
	return newCursor(s)
}

func (d *dsImpl) Run(fq *ds.FinalizedQuery, cb ds.RawRunCB) error {
	idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
	return executeQuery(fq, d.ns, false, idx, head, cb)
}

func (d *dsImpl) AddIndexes(idxs ...*ds.IndexDefinition) {
	if len(idxs) == 0 {
		return
	}

	for _, i := range idxs {
		if !i.Compound() {
			panic(fmt.Errorf("Attempted to add non-compound index: %s", i))
		}
	}

	d.data.Lock()
	defer d.data.Unlock()
	addIndex(d.data.head, d.ns, idxs)
}

func (d *dsImpl) TakeIndexSnapshot() ds.TestingSnapshot {
	return d.data.takeSnapshot()
}

func (d *dsImpl) SetIndexSnapshot(snap ds.TestingSnapshot) {
	d.data.setSnapshot(snap.(*memStore))
}

func (d *dsImpl) CatchupIndexes() {
	d.data.catchupIndexes()
}

func (d *dsImpl) SetTransactionRetryCount(count int) {
	d.data.txnFakeRetry = count
}

func (d *dsImpl) Testable() ds.Testable {
	return d
}

////////////////////////////////// txnDsImpl ///////////////////////////////////

type txnDsImpl struct {
	data *txnDataStoreData
	ns   string
}

var _ ds.RawInterface = (*txnDsImpl)(nil)

func (d *txnDsImpl) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) error {
	return d.data.run(func() error {
		d.data.putMulti(keys, vals, cb)
		return nil
	})
}

func (d *txnDsImpl) GetMulti(keys []*ds.Key, _meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	return d.data.run(func() error {
		return d.data.getMulti(keys, cb)
	})
}

func (d *txnDsImpl) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	return d.data.run(func() error {
		return d.data.delMulti(keys, cb)
	})
}

func (d *txnDsImpl) DecodeCursor(s string) (ds.Cursor, error) {
	return newCursor(s)
}

func (d *txnDsImpl) Run(q *ds.FinalizedQuery, cb ds.RawRunCB) error {
	return executeQuery(q, d.ns, true, d.data.snap, d.data.snap, cb)
}

func (*txnDsImpl) RunInTransaction(func(c context.Context) error, *ds.TransactionOptions) error {
	return errors.New("datastore: nested transactions are not supported")
}

func (*txnDsImpl) Testable() ds.Testable {
	return nil
}
