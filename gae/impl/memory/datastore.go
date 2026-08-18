// Copyright 2015 The LUCI Authors.
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

package memory

import (
	"context"
	"errors"
	"fmt"

	ds "go.chromium.org/luci/gae/service/datastore"
)

//////////////////////////////////// public ////////////////////////////////////

// useRDS adds a gae.Datastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return ds.SetRawFactory(c, func(ic context.Context) ds.RawInterface {
		kc := ds.GetKeyContext(ic)
		memCtx, isTxn := cur(ic)
		dsd := memCtx.Get(memContextDSIdx)
		if isTxn {
			return &txnDsImpl{ic, dsd.(*txnDataStoreData), kc}
		}
		return &dsImpl{ic, dsd.(*dataStoreData), kc}
	})
}

//////////////////////////////////// dsImpl ////////////////////////////////////

// dsImpl exists solely to bind the current c to the datastore data.
type dsImpl struct {
	context.Context

	data *dataStoreData
	kc   ds.KeyContext
}

var _ ds.RawInterface = (*dsImpl)(nil)

func (d *dsImpl) AllocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	return d.data.allocateIDs(keys, cb)
}

func (d *dsImpl) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	d.data.putMulti(keys, vals, cb, false)
	return nil
}

func (d *dsImpl) GetMulti(keys []*ds.Key, _meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	return d.data.getMulti(keys, cb)
}

func (d *dsImpl) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	d.data.delMulti(keys, cb, false)
	return nil
}

func (d *dsImpl) DecodeCursor(s string) (ds.RawCursor, error) {
	return newCursor(s)
}

func (d *dsImpl) RunQuery(fq *ds.FinalizedQuery) ds.RawQueryIter {
	idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
	return d.data.stripSpecialPropsIter(executeQuery(fq, d.kc, d.data, false, idx, head))
}

func (d *dsImpl) Count(fq *ds.FinalizedQuery) (ret int64, err error) {
	idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
	ret, err = countQuery(fq, d.kc, d.data, false, idx, head)
	if d.data.maybeAutoIndex(err) {
		idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
		ret, err = countQuery(fq, d.kc, d.data, false, idx, head)
	}
	return
}

func (d *dsImpl) WithoutTransaction() context.Context {
	// Already not in a Transaction.
	return d
}

func (*dsImpl) CurrentTransaction() ds.Transaction { return nil }

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

func (d *dsImpl) Constraints() ds.Constraints { return d.data.getConstraints() }

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

func (d *dsImpl) ShowSpecialProperties(show bool) {
	d.data.setShowSpecialProperties(show)
}

func (d *dsImpl) SetConstraints(c *ds.Constraints) error {
	if c == nil {
		c = &ds.Constraints{}
	}
	d.data.setConstraints(*c)
	return nil
}

func (d *dsImpl) GetTestable() ds.Testable { return d }

////////////////////////////////// txnDsImpl ///////////////////////////////////

type txnDsImpl struct {
	context.Context

	data *txnDataStoreData
	kc   ds.KeyContext
}

var _ ds.RawInterface = (*txnDsImpl)(nil)

func (d *txnDsImpl) AllocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	return d.data.parent.allocateIDs(keys, cb)
}

func (d *txnDsImpl) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
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

func (d *txnDsImpl) DecodeCursor(s string) (ds.RawCursor, error) { return newCursor(s) }

func (d *txnDsImpl) RunQuery(q *ds.FinalizedQuery) ds.RawQueryIter {
	return d.data.parent.stripSpecialPropsIter(executeQuery(q, d.kc, nil, true, d.data.snap, d.data.snap))
}

func (d *txnDsImpl) Count(fq *ds.FinalizedQuery) (ret int64, err error) {
	return countQuery(fq, d.kc, nil, true, d.data.snap, d.data.snap)
}

func (*txnDsImpl) RunInTransaction(func(c context.Context) error, *ds.TransactionOptions) error {
	return errors.New("datastore: nested transactions are not supported")
}

func (d *txnDsImpl) WithoutTransaction() context.Context {
	return context.WithValue(d, &currentTxnKey, nil)
}

func (d *txnDsImpl) CurrentTransaction() ds.Transaction {
	return d.data.txn
}

func (d *txnDsImpl) Constraints() ds.Constraints { return d.data.parent.getConstraints() }

func (d *txnDsImpl) GetTestable() ds.Testable { return nil }
