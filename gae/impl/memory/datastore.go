// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"

	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
)

//////////////////////////////////// public ////////////////////////////////////

// useRDS adds a gae.Datastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return ds.SetRawFactory(c, func(ic context.Context, wantTxn bool) ds.RawInterface {
		ns, hasNS := curGID(ic).getNamespace()
		maybeTxnCtx := cur(ic)

		needResetCtx := false
		if !wantTxn {
			rootctx := curNoTxn(ic)
			if rootctx != maybeTxnCtx {
				needResetCtx = true
				maybeTxnCtx = rootctx
			}
		}

		dsd := maybeTxnCtx.Get(memContextDSIdx)
		if x, ok := dsd.(*dataStoreData); ok {
			if needResetCtx {
				ic = context.WithValue(ic, memContextKey, maybeTxnCtx)
			}
			return &dsImpl{x, ns, hasNS, ic}
		}
		return &txnDsImpl{dsd.(*txnDataStoreData), ns, hasNS}
	})
}

// NewDatastore creates a new standalone memory implementation of the datastore,
// suitable for embedding for doing in-memory data organization.
//
// It's configured by default with the following settings:
//   * AutoIndex(true)
//   * Consistent(true)
//   * DisableSpecialEntities(true)
//
// These settings can of course be changed by using the Testable() interface.
func NewDatastore(aid, ns string) (ds.Interface, error) {
	ctx := UseWithAppID(context.Background(), aid)

	if ns != "" {
		var err error
		ctx, err = info.Get(ctx).Namespace(ns)
		if err != nil {
			return nil, err
		}
	}

	ret := ds.Get(ctx)
	t := ret.Testable()
	t.AutoIndex(true)
	t.Consistent(true)
	t.DisableSpecialEntities(true)
	return ret, nil
}

//////////////////////////////////// dsImpl ////////////////////////////////////

// dsImpl exists solely to bind the current c to the datastore data.
type dsImpl struct {
	data  *dataStoreData
	ns    string
	hasNS bool
	c     context.Context
}

var _ ds.RawInterface = (*dsImpl)(nil)

func (d *dsImpl) AllocateIDs(incomplete *ds.Key, n int) (int64, error) {
	return d.data.allocateIDs(incomplete, n)
}

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
	if err := assertQueryNamespace(d.ns, d.hasNS); err != nil {
		return err
	}

	idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
	err := executeQuery(fq, d.data.aid, d.ns, false, idx, head, cb)
	if d.data.maybeAutoIndex(err) {
		idx, head = d.data.getQuerySnaps(!fq.EventuallyConsistent())
		err = executeQuery(fq, d.data.aid, d.ns, false, idx, head, cb)
	}
	return err
}

func (d *dsImpl) Count(fq *ds.FinalizedQuery) (ret int64, err error) {
	if err := assertQueryNamespace(d.ns, d.hasNS); err != nil {
		return 0, err
	}

	idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
	ret, err = countQuery(fq, d.data.aid, d.ns, false, idx, head)
	if d.data.maybeAutoIndex(err) {
		idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
		ret, err = countQuery(fq, d.data.aid, d.ns, false, idx, head)
	}
	return
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

	d.data.addIndexes(idxs)
}

func (d *dsImpl) TakeIndexSnapshot() ds.TestingSnapshot {
	return d.data.takeSnapshot()
}

func (d *dsImpl) SetIndexSnapshot(snap ds.TestingSnapshot) {
	d.data.setSnapshot(snap.(memStore))
}

func (d *dsImpl) CatchupIndexes() {
	d.data.catchupIndexes()
}

func (d *dsImpl) SetTransactionRetryCount(count int) {
	d.data.setTxnRetry(count)
}

func (d *dsImpl) Consistent(always bool) {
	d.data.setConsistent(always)
}

func (d *dsImpl) AutoIndex(enable bool) {
	d.data.setAutoIndex(enable)
}

func (d *dsImpl) DisableSpecialEntities(enabled bool) {
	d.data.setDisableSpecialEntities(enabled)
}

func (d *dsImpl) Testable() ds.Testable {
	return d
}

////////////////////////////////// txnDsImpl ///////////////////////////////////

type txnDsImpl struct {
	data  *txnDataStoreData
	ns    string
	hasNS bool
}

var _ ds.RawInterface = (*txnDsImpl)(nil)

func (d *txnDsImpl) AllocateIDs(incomplete *ds.Key, n int) (int64, error) {
	return d.data.parent.allocateIDs(incomplete, n)
}

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
	if err := assertQueryNamespace(d.ns, d.hasNS); err != nil {
		return err
	}

	// note that autoIndex has no effect inside transactions. This is because
	// the transaction guarantees a consistent view of head at the time that the
	// transaction opens. At best, we could add the index on head, but then return
	// the error anyway, but adding the index then re-snapping at head would
	// potentially reveal other entities not in the original transaction snapshot.
	//
	// It's possible that if you have full-consistency and also auto index enabled
	// that this would make sense... but at that point you should probably just
	// add the index up front.
	return executeQuery(q, d.data.parent.aid, d.ns, true, d.data.snap, d.data.snap, cb)
}

func (d *txnDsImpl) Count(fq *ds.FinalizedQuery) (ret int64, err error) {
	if err := assertQueryNamespace(d.ns, d.hasNS); err != nil {
		return 0, err
	}

	return countQuery(fq, d.data.parent.aid, d.ns, true, d.data.snap, d.data.snap)
}

func (*txnDsImpl) RunInTransaction(func(c context.Context) error, *ds.TransactionOptions) error {
	return errors.New("datastore: nested transactions are not supported")
}

func (*txnDsImpl) Testable() ds.Testable {
	return nil
}

func assertQueryNamespace(ns string, hasNS bool) error {
	if ns == "" && hasNS {
		// The user has set an empty namespace. Datastore does not support this
		// for queries.
		//
		// Bug on file is:
		// https://code.google.com/p/googleappengine/issues/detail?id=12914
		return errors.New("namespace may not be present and empty")
	}
	return nil
}
