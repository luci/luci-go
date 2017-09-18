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
	"errors"
	"fmt"

	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
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

// NewDatastore creates a new standalone memory implementation of the datastore,
// suitable for embedding for doing in-memory data organization.
//
// It's configured by default with the following settings:
//   * AutoIndex(true)
//   * Consistent(true)
//   * DisableSpecialEntities(true)
//
// These settings can of course be changed by using the Testable interface.
func NewDatastore(c context.Context, inf info.RawInterface) ds.RawInterface {
	kc := ds.GetKeyContext(c)

	memctx := newMemContext(kc.AppID)

	dsCtx := info.Set(context.Background(), inf)
	rds := &dsImpl{dsCtx, memctx.Get(memContextDSIdx).(*dataStoreData), kc}

	ret := ds.Raw(ds.SetRaw(dsCtx, rds))
	t := ret.GetTestable()
	t.AutoIndex(true)
	t.Consistent(true)
	t.DisableSpecialEntities(true)

	return ret
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

func (d *dsImpl) DecodeCursor(s string) (ds.Cursor, error) {
	return newCursor(s)
}

func (d *dsImpl) Run(fq *ds.FinalizedQuery, cb ds.RawRunCB) error {
	idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
	err := executeQuery(fq, d.kc, false, idx, head, cb)
	if d.data.maybeAutoIndex(err) {
		idx, head = d.data.getQuerySnaps(!fq.EventuallyConsistent())
		err = executeQuery(fq, d.kc, false, idx, head, cb)
	}
	return err
}

func (d *dsImpl) Count(fq *ds.FinalizedQuery) (ret int64, err error) {
	idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
	ret, err = countQuery(fq, d.kc, false, idx, head)
	if d.data.maybeAutoIndex(err) {
		idx, head := d.data.getQuerySnaps(!fq.EventuallyConsistent())
		ret, err = countQuery(fq, d.kc, false, idx, head)
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

func (d *dsImpl) DisableSpecialEntities(enabled bool) {
	d.data.setDisableSpecialEntities(enabled)
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

func (d *txnDsImpl) DecodeCursor(s string) (ds.Cursor, error) { return newCursor(s) }

func (d *txnDsImpl) Run(q *ds.FinalizedQuery, cb ds.RawRunCB) error {
	// note that autoIndex has no effect inside transactions. This is because
	// the transaction guarantees a consistent view of head at the time that the
	// transaction opens. At best, we could add the index on head, but then return
	// the error anyway, but adding the index then re-snapping at head would
	// potentially reveal other entities not in the original transaction snapshot.
	//
	// It's possible that if you have full-consistency and also auto index enabled
	// that this would make sense... but at that point you should probably just
	// add the index up front.
	return executeQuery(q, d.kc, true, d.data.snap, d.data.snap, cb)
}

func (d *txnDsImpl) Count(fq *ds.FinalizedQuery) (ret int64, err error) {
	return countQuery(fq, d.kc, true, d.data.snap, d.data.snap)
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
