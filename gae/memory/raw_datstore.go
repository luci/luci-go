// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"
	"golang.org/x/net/context"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/dummy"
	"infra/gae/libs/gae/helper"
)

//////////////////////////////////// public ////////////////////////////////////

// useRDS adds a gae.Datastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return gae.SetRDSFactory(c, func(ic context.Context) gae.RawDatastore {
		dsd := cur(ic).Get(memContextDSIdx)

		switch x := dsd.(type) {
		case *dataStoreData:
			return &dsImpl{dummy.RDS(), x, curGID(ic).namespace, ic}
		case *txnDataStoreData:
			return &txnDsImpl{dummy.RDS(), x, curGID(ic).namespace}
		default:
			panic(fmt.Errorf("DS: bad type: %v in context %v", dsd, ic))
		}
	})
}

//////////////////////////////////// dsImpl ////////////////////////////////////

// dsImpl exists solely to bind the current c to the datastore data.
type dsImpl struct {
	gae.RawDatastore

	data *dataStoreData
	ns   string
	c    context.Context
}

var _ gae.RawDatastore = (*dsImpl)(nil)

func (d *dsImpl) NewKey(kind, stringID string, intID int64, parent gae.DSKey) gae.DSKey {
	return helper.NewDSKey(globalAppID, d.ns, kind, stringID, intID, parent)
}

func (d *dsImpl) Put(key gae.DSKey, pls gae.DSPropertyLoadSaver) (retKey gae.DSKey, err error) {
	return d.data.put(d.ns, key, pls)
}

func (d *dsImpl) Get(key gae.DSKey, pls gae.DSPropertyLoadSaver) error {
	return d.data.get(d.ns, key, pls)
}

func (d *dsImpl) Delete(key gae.DSKey) error {
	return d.data.del(d.ns, key)
}

////////////////////////////////// txnDsImpl ///////////////////////////////////

type txnDsImpl struct {
	gae.RawDatastore

	data *txnDataStoreData
	ns   string
}

var _ gae.RawDatastore = (*txnDsImpl)(nil)

func (d *dsImpl) NewQuery(kind string) gae.DSQuery {
	return &queryImpl{DSQuery: dummy.QY(), ns: d.ns, kind: kind}
}

func (d *dsImpl) Run(q gae.DSQuery) gae.RDSIterator {
	rq := q.(*queryImpl)
	rq = rq.normalize().checkCorrectness(d.ns, false)
	return &queryIterImpl{rq}
}

func (d *dsImpl) GetAll(q gae.DSQuery, dst *[]gae.DSPropertyMap) ([]gae.DSKey, error) {
	// TODO(riannucci): assert that dst is a slice of structs
	return nil, nil
}

func (d *dsImpl) Count(q gae.DSQuery) (ret int, err error) {
	itr := d.Run(q.KeysOnly())
	for _, err = itr.Next(nil); err != nil; _, err = itr.Next(nil) {
		ret++
	}
	if err == gae.ErrDSQueryDone {
		err = nil
	}
	return
}

func (d *txnDsImpl) NewKey(kind, stringID string, intID int64, parent gae.DSKey) gae.DSKey {
	return helper.NewDSKey(globalAppID, d.ns, kind, stringID, intID, parent)
}

func (d *txnDsImpl) Put(key gae.DSKey, pls gae.DSPropertyLoadSaver) (gae.DSKey, error) {
	if err := d.data.isBroken(); err != nil {
		return nil, err
	}
	return d.data.put(d.ns, key, pls)
}

func (d *txnDsImpl) Get(key gae.DSKey, pls gae.DSPropertyLoadSaver) error {
	if err := d.data.isBroken(); err != nil {
		return err
	}
	return d.data.get(d.ns, key, pls)
}

func (d *txnDsImpl) Delete(key gae.DSKey) error {
	if err := d.data.isBroken(); err != nil {
		return err
	}
	return d.data.del(d.ns, key)
}

func (*txnDsImpl) RunInTransaction(func(c context.Context) error, *gae.DSTransactionOptions) error {
	return errors.New("datastore: nested transactions are not supported")
}
