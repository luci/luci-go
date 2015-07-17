// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"golang.org/x/net/context"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/helper"
)

//////////////////////////////////// public ////////////////////////////////////

// useRDS adds a gae.Datastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return gae.SetRDSFactory(c, func(ic context.Context) gae.RawDatastore {
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

var _ gae.RawDatastore = (*dsImpl)(nil)

func (d *dsImpl) DecodeKey(encoded string) (gae.DSKey, error) {
	return helper.NewDSKeyFromEncoded(encoded)
}

func (d *dsImpl) NewKey(kind, stringID string, intID int64, parent gae.DSKey) gae.DSKey {
	return helper.NewDSKey(globalAppID, d.ns, kind, stringID, intID, parent)
}

func (d *dsImpl) Put(key gae.DSKey, pls gae.DSPropertyLoadSaver) (gae.DSKey, error) {
	return d.data.put(d.ns, key, pls)
}

func (d *dsImpl) PutMulti(keys []gae.DSKey, plss []gae.DSPropertyLoadSaver) ([]gae.DSKey, error) {
	return d.data.putMulti(d.ns, keys, plss)
}

func (d *dsImpl) Get(key gae.DSKey, pls gae.DSPropertyLoadSaver) error {
	return d.data.get(d.ns, key, pls)
}

func (d *dsImpl) GetMulti(keys []gae.DSKey, plss []gae.DSPropertyLoadSaver) error {
	return d.data.getMulti(d.ns, keys, plss)
}

func (d *dsImpl) Delete(key gae.DSKey) error {
	return d.data.del(d.ns, key)
}

func (d *dsImpl) DeleteMulti(keys []gae.DSKey) error {
	return d.data.delMulti(d.ns, keys)
}

func (d *dsImpl) NewQuery(kind string) gae.DSQuery {
	return &queryImpl{ns: d.ns, kind: kind}
}

func (d *dsImpl) Run(q gae.DSQuery) gae.RDSIterator {
	rq := q.(*queryImpl)
	rq = rq.normalize().checkCorrectness(d.ns, false)
	return &queryIterImpl{rq}
}

func (d *dsImpl) GetAll(q gae.DSQuery, dst *[]gae.DSPropertyMap) ([]gae.DSKey, error) {
	// TODO(riannucci): assert that dst is a slice of structs
	panic("NOT IMPLEMENTED")
}

func (d *dsImpl) Count(q gae.DSQuery) (int, error) {
	return count(d.Run(q.KeysOnly()))
}

////////////////////////////////// txnDsImpl ///////////////////////////////////

type txnDsImpl struct {
	data *txnDataStoreData
	ns   string
}

var _ gae.RawDatastore = (*txnDsImpl)(nil)

func (d *txnDsImpl) DecodeKey(encoded string) (gae.DSKey, error) {
	return helper.NewDSKeyFromEncoded(encoded)
}

func (d *txnDsImpl) NewKey(kind, stringID string, intID int64, parent gae.DSKey) gae.DSKey {
	return helper.NewDSKey(globalAppID, d.ns, kind, stringID, intID, parent)
}

func (d *txnDsImpl) Put(key gae.DSKey, pls gae.DSPropertyLoadSaver) (retKey gae.DSKey, err error) {
	err = d.data.run(func() (err error) {
		retKey, err = d.data.put(d.ns, key, pls)
		return
	})
	return
}

func (d *txnDsImpl) PutMulti(keys []gae.DSKey, plss []gae.DSPropertyLoadSaver) (retKeys []gae.DSKey, err error) {
	err = d.data.run(func() (err error) {
		retKeys, err = d.data.putMulti(d.ns, keys, plss)
		return
	})
	return
}

func (d *txnDsImpl) Get(key gae.DSKey, pls gae.DSPropertyLoadSaver) error {
	return d.data.run(func() error {
		return d.data.get(d.ns, key, pls)
	})
}

func (d *txnDsImpl) GetMulti(keys []gae.DSKey, plss []gae.DSPropertyLoadSaver) error {
	return d.data.run(func() error {
		return d.data.getMulti(d.ns, keys, plss)
	})
}

func (d *txnDsImpl) Delete(key gae.DSKey) error {
	return d.data.run(func() error {
		return d.data.del(d.ns, key)
	})
}

func (d *txnDsImpl) DeleteMulti(keys []gae.DSKey) error {
	return d.data.run(func() error {
		return d.data.delMulti(d.ns, keys)
	})
}

func (d *txnDsImpl) Run(q gae.DSQuery) gae.RDSIterator {
	rq := q.(*queryImpl)
	if rq.ancestor == nil {
		rq.err = errors.New("memory: queries in transactions only support ancestor queries")
		return &queryIterImpl{rq}
	}
	panic("NOT IMPLEMENTED")
}

func (*txnDsImpl) RunInTransaction(func(c context.Context) error, *gae.DSTransactionOptions) error {
	return errors.New("datastore: nested transactions are not supported")
}

func (d *txnDsImpl) NewQuery(kind string) gae.DSQuery {
	return &queryImpl{ns: d.ns, kind: kind}
}

func (d *txnDsImpl) GetAll(q gae.DSQuery, dst *[]gae.DSPropertyMap) ([]gae.DSKey, error) {
	// TODO(riannucci): assert that dst is a slice of structs
	panic("NOT IMPLEMENTED")
}

func (d *txnDsImpl) Count(q gae.DSQuery) (int, error) {
	return count(d.Run(q.KeysOnly()))
}

func count(itr gae.RDSIterator) (ret int, err error) {
	for _, err = itr.Next(nil); err != nil; _, err = itr.Next(nil) {
		ret++
	}
	if err == gae.ErrDSQueryDone {
		err = nil
	}
	return
}
