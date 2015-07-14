// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"
	"golang.org/x/net/context"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/helper"
)

//////////////////////////////////// public ////////////////////////////////////

// useDS adds a gae.Datastore implementation to context, accessible
// by gae.GetDS(c)
func useDS(c context.Context) context.Context {
	return gae.SetRDSFactory(c, func(ic context.Context) gae.RawDatastore {
		dsd := cur(ic).Get(memContextDSIdx)

		switch x := dsd.(type) {
		case *dataStoreData:
			return &dsImpl{gae.DummyRDS(), x, curGID(ic).namespace, ic}
		case *txnDataStoreData:
			return &txnDsImpl{gae.DummyRDS(), x, curGID(ic).namespace}
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

var _ interface {
	gae.RawDatastore
	gae.Testable
} = (*dsImpl)(nil)

func (d *dsImpl) BreakFeatures(err error, features ...string) {
	d.data.BreakFeatures(err, features...)
}
func (d *dsImpl) UnbreakFeatures(features ...string) {
	d.data.UnbreakFeatures(features...)
}

func (d *dsImpl) NewKey(kind, stringID string, intID int64, parent gae.DSKey) gae.DSKey {
	return helper.NewDSKey(globalAppID, d.ns, kind, stringID, intID, parent)
}

func (d *dsImpl) Put(key gae.DSKey, src interface{}) (retKey gae.DSKey, err error) {
	err = d.data.RunIfNotBroken(func() (err error) {
		retKey, err = d.data.put(d.ns, key, src)
		return
	})
	return
}

func (d *dsImpl) Get(key gae.DSKey, dst interface{}) error {
	return d.data.RunIfNotBroken(func() error {
		return d.data.get(d.ns, key, dst)
	})
}

func (d *dsImpl) Delete(key gae.DSKey) error {
	return d.data.RunIfNotBroken(func() error {
		return d.data.del(d.ns, key)
	})
}

////////////////////////////////// txnDsImpl ///////////////////////////////////

type txnDsImpl struct {
	gae.RawDatastore

	data *txnDataStoreData
	ns   string
}

var (
	_ = gae.RawDatastore((*txnDsImpl)(nil))
	_ = gae.Testable((*txnDsImpl)(nil))
)

func (d *dsImpl) NewQuery(kind string) gae.DSQuery {
	return &queryImpl{DSQuery: gae.DummyQY(), ns: d.ns, kind: kind}
}

func (d *dsImpl) Run(q gae.DSQuery) gae.DSIterator {
	rq := q.(*queryImpl)
	rq = rq.normalize().checkCorrectness(d.ns, false)
	return &queryIterImpl{rq}
}

func (d *dsImpl) GetAll(q gae.DSQuery, dst interface{}) ([]gae.DSKey, error) {
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

func (d *txnDsImpl) BreakFeatures(err error, features ...string) {
	d.data.BreakFeatures(err, features...)
}
func (d *txnDsImpl) UnbreakFeatures(features ...string) {
	d.data.UnbreakFeatures(features...)
}

func (d *txnDsImpl) NewKey(kind, stringID string, intID int64, parent gae.DSKey) gae.DSKey {
	return helper.NewDSKey(globalAppID, d.ns, kind, stringID, intID, parent)
}

func (d *txnDsImpl) Put(key gae.DSKey, src interface{}) (retKey gae.DSKey, err error) {
	err = d.data.RunIfNotBroken(func() (err error) {
		retKey, err = d.data.put(d.ns, key, src)
		return
	})
	return
}

func (d *txnDsImpl) Get(key gae.DSKey, dst interface{}) error {
	return d.data.RunIfNotBroken(func() error {
		return d.data.get(d.ns, key, dst)
	})
}

func (d *txnDsImpl) Delete(key gae.DSKey) error {
	return d.data.RunIfNotBroken(func() error {
		return d.data.del(d.ns, key)
	})
}

func (*txnDsImpl) RunInTransaction(func(c context.Context) error, *gae.DSTransactionOptions) error {
	return errors.New("datastore: nested transactions are not supported")
}
