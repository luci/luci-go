// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"

	"golang.org/x/net/context"

	rds "github.com/luci/gae/service/rawdatastore"
)

//////////////////////////////////// public ////////////////////////////////////

// useRDS adds a gae.Datastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return rds.SetFactory(c, func(ic context.Context) rds.Interface {
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

var _ rds.Interface = (*dsImpl)(nil)

func (d *dsImpl) DecodeKey(encoded string) (rds.Key, error) {
	return rds.NewKeyFromEncoded(encoded)
}

func (d *dsImpl) NewKey(kind, stringID string, intID int64, parent rds.Key) rds.Key {
	return rds.NewKey(globalAppID, d.ns, kind, stringID, intID, parent)
}

func (d *dsImpl) PutMulti(keys []rds.Key, vals []rds.PropertyLoadSaver, cb rds.PutMultiCB) error {
	d.data.putMulti(keys, vals, cb)
	return nil
}

func (d *dsImpl) GetMulti(keys []rds.Key, cb rds.GetMultiCB) error {
	d.data.getMulti(keys, cb)
	return nil
}

func (d *dsImpl) DeleteMulti(keys []rds.Key, cb rds.DeleteMultiCB) error {
	d.data.delMulti(keys, cb)
	return nil
}

func (d *dsImpl) NewQuery(kind string) rds.Query {
	return &queryImpl{ns: d.ns, kind: kind}
}

func (d *dsImpl) Run(q rds.Query, cb rds.RunCB) error {
	return nil
	/*
		rq := q.(*queryImpl)
		rq = rq.normalize().checkCorrectness(d.ns, false)
		return &queryIterImpl{rq}
	*/
}

////////////////////////////////// txnDsImpl ///////////////////////////////////

type txnDsImpl struct {
	data *txnDataStoreData
	ns   string
}

var _ rds.Interface = (*txnDsImpl)(nil)

func (d *txnDsImpl) DecodeKey(encoded string) (rds.Key, error) {
	return rds.NewKeyFromEncoded(encoded)
}

func (d *txnDsImpl) NewKey(kind, stringID string, intID int64, parent rds.Key) rds.Key {
	return rds.NewKey(globalAppID, d.ns, kind, stringID, intID, parent)
}

func (d *txnDsImpl) PutMulti(keys []rds.Key, vals []rds.PropertyLoadSaver, cb rds.PutMultiCB) error {
	return d.data.run(func() error {
		d.data.putMulti(keys, vals, cb)
		return nil
	})
}

func (d *txnDsImpl) GetMulti(keys []rds.Key, cb rds.GetMultiCB) error {
	return d.data.run(func() error {
		return d.data.getMulti(keys, cb)
	})
}

func (d *txnDsImpl) DeleteMulti(keys []rds.Key, cb rds.DeleteMultiCB) error {
	return d.data.run(func() error {
		return d.data.delMulti(keys, cb)
	})
}

func (d *txnDsImpl) Run(q rds.Query, cb rds.RunCB) error {
	rq := q.(*queryImpl)
	if rq.ancestor == nil {
		return errors.New("memory: queries in transactions only support ancestor queries")
	}
	panic("NOT IMPLEMENTED")
}

func (*txnDsImpl) RunInTransaction(func(c context.Context) error, *rds.TransactionOptions) error {
	return errors.New("datastore: nested transactions are not supported")
}

func (d *txnDsImpl) NewQuery(kind string) rds.Query {
	return &queryImpl{ns: d.ns, kind: kind}
}
