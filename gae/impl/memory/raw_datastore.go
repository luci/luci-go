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

func (d *dsImpl) Put(key rds.Key, pls rds.PropertyLoadSaver) (rds.Key, error) {
	return d.data.put(d.ns, key, pls)
}

func (d *dsImpl) PutMulti(keys []rds.Key, plss []rds.PropertyLoadSaver) ([]rds.Key, error) {
	return d.data.putMulti(d.ns, keys, plss)
}

func (d *dsImpl) Get(key rds.Key, pls rds.PropertyLoadSaver) error {
	return d.data.get(d.ns, key, pls)
}

func (d *dsImpl) GetMulti(keys []rds.Key, plss []rds.PropertyLoadSaver) error {
	return d.data.getMulti(d.ns, keys, plss)
}

func (d *dsImpl) Delete(key rds.Key) error {
	return d.data.del(d.ns, key)
}

func (d *dsImpl) DeleteMulti(keys []rds.Key) error {
	return d.data.delMulti(d.ns, keys)
}

func (d *dsImpl) NewQuery(kind string) rds.Query {
	return &queryImpl{ns: d.ns, kind: kind}
}

func (d *dsImpl) Run(q rds.Query) rds.Iterator {
	rq := q.(*queryImpl)
	rq = rq.normalize().checkCorrectness(d.ns, false)
	return &queryIterImpl{rq}
}

func (d *dsImpl) GetAll(q rds.Query, dst *[]rds.PropertyMap) ([]rds.Key, error) {
	// TODO(riannucci): assert that dst is a slice of structs
	panic("NOT IMPLEMENTED")
}

func (d *dsImpl) Count(q rds.Query) (int, error) {
	return count(d.Run(q.KeysOnly()))
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

func (d *txnDsImpl) Put(key rds.Key, pls rds.PropertyLoadSaver) (retKey rds.Key, err error) {
	err = d.data.run(func() (err error) {
		retKey, err = d.data.put(d.ns, key, pls)
		return
	})
	return
}

func (d *txnDsImpl) PutMulti(keys []rds.Key, plss []rds.PropertyLoadSaver) (retKeys []rds.Key, err error) {
	err = d.data.run(func() (err error) {
		retKeys, err = d.data.putMulti(d.ns, keys, plss)
		return
	})
	return
}

func (d *txnDsImpl) Get(key rds.Key, pls rds.PropertyLoadSaver) error {
	return d.data.run(func() error {
		return d.data.get(d.ns, key, pls)
	})
}

func (d *txnDsImpl) GetMulti(keys []rds.Key, plss []rds.PropertyLoadSaver) error {
	return d.data.run(func() error {
		return d.data.getMulti(d.ns, keys, plss)
	})
}

func (d *txnDsImpl) Delete(key rds.Key) error {
	return d.data.run(func() error {
		return d.data.del(d.ns, key)
	})
}

func (d *txnDsImpl) DeleteMulti(keys []rds.Key) error {
	return d.data.run(func() error {
		return d.data.delMulti(d.ns, keys)
	})
}

func (d *txnDsImpl) Run(q rds.Query) rds.Iterator {
	rq := q.(*queryImpl)
	if rq.ancestor == nil {
		rq.err = errors.New("memory: queries in transactions only support ancestor queries")
		return &queryIterImpl{rq}
	}
	panic("NOT IMPLEMENTED")
}

func (*txnDsImpl) RunInTransaction(func(c context.Context) error, *rds.TransactionOptions) error {
	return errors.New("datastore: nested transactions are not supported")
}

func (d *txnDsImpl) NewQuery(kind string) rds.Query {
	return &queryImpl{ns: d.ns, kind: kind}
}

func (d *txnDsImpl) GetAll(q rds.Query, dst *[]rds.PropertyMap) ([]rds.Key, error) {
	// TODO(riannucci): assert that dst is a slice of structs
	panic("NOT IMPLEMENTED")
}

func (d *txnDsImpl) Count(q rds.Query) (int, error) {
	return count(d.Run(q.KeysOnly()))
}

func count(itr rds.Iterator) (ret int, err error) {
	for _, err = itr.Next(nil); err != nil; _, err = itr.Next(nil) {
		ret++
	}
	if err == rds.ErrQueryDone {
		err = nil
	}
	return
}
