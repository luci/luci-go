// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"golang.org/x/net/context"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/helper"

	"google.golang.org/appengine/datastore"
)

// useRDS adds a gae.RawDatastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return gae.SetRDSFactory(c, func(ci context.Context) gae.RawDatastore {
		return rdsImpl{ci}
	})
}

////////// Query

type queryImpl struct{ *datastore.Query }

func (q queryImpl) Distinct() gae.DSQuery {
	return queryImpl{q.Query.Distinct()}
}
func (q queryImpl) End(c gae.DSCursor) gae.DSQuery {
	return queryImpl{q.Query.End(c.(datastore.Cursor))}
}
func (q queryImpl) EventualConsistency() gae.DSQuery {
	return queryImpl{q.Query.EventualConsistency()}
}
func (q queryImpl) KeysOnly() gae.DSQuery {
	return queryImpl{q.Query.KeysOnly()}
}
func (q queryImpl) Limit(limit int) gae.DSQuery {
	return queryImpl{q.Query.Limit(limit)}
}
func (q queryImpl) Offset(offset int) gae.DSQuery {
	return queryImpl{q.Query.Offset(offset)}
}
func (q queryImpl) Order(fieldName string) gae.DSQuery {
	return queryImpl{q.Query.Order(fieldName)}
}
func (q queryImpl) Start(c gae.DSCursor) gae.DSQuery {
	return queryImpl{q.Query.Start(c.(datastore.Cursor))}
}
func (q queryImpl) Ancestor(ancestor gae.DSKey) gae.DSQuery {
	return queryImpl{q.Query.Ancestor(dsF2R(ancestor))}
}
func (q queryImpl) Project(fieldNames ...string) gae.DSQuery {
	return queryImpl{q.Query.Project(fieldNames...)}
}
func (q queryImpl) Filter(filterStr string, value interface{}) gae.DSQuery {
	return queryImpl{q.Query.Filter(filterStr, value)}
}

////////// Iterator

type iteratorImpl struct{ *datastore.Iterator }

var _ gae.DSIterator = iteratorImpl{}

func (i iteratorImpl) Cursor() (gae.DSCursor, error) {
	return i.Iterator.Cursor()
}

func (i iteratorImpl) Next(dst interface{}) (gae.DSKey, error) {
	return dsR2FErr(i.Iterator.Next(dst))
}

////////// Datastore

type rdsImpl struct{ context.Context }

// NewKeyer
func (d rdsImpl) NewKey(kind, stringID string, intID int64, parent gae.DSKey) gae.DSKey {
	return dsR2F(datastore.NewKey(d, kind, stringID, intID, dsF2R(parent)))
}

func (rdsImpl) DecodeKey(encoded string) (gae.DSKey, error) {
	return dsR2FErr(datastore.DecodeKey(encoded))
}

func multiWrap(os interface{}) ([]datastore.PropertyLoadSaver, error) {
	plss, err := helper.MultiGetPLS(os)
	if err != nil {
		return nil, err
	}

	ret := make([]datastore.PropertyLoadSaver, len(plss))
	for i, pls := range plss {
		ret[i] = &typeFilter{pls}
	}
	return ret, nil
}

func (d rdsImpl) Delete(k gae.DSKey) error { return datastore.Delete(d, dsF2R(k)) }
func (d rdsImpl) Get(key gae.DSKey, dst interface{}) error {
	pls, err := helper.GetPLS(dst)
	if err != nil {
		return err
	}
	return datastore.Get(d, dsF2R(key), &typeFilter{pls})
}
func (d rdsImpl) Put(key gae.DSKey, src interface{}) (gae.DSKey, error) {
	pls, err := helper.GetPLS(src)
	if err != nil {
		return nil, err
	}
	return dsR2FErr(datastore.Put(d, dsF2R(key), &typeFilter{pls}))
}

func (d rdsImpl) DeleteMulti(ks []gae.DSKey) error {
	return gae.FixError(datastore.DeleteMulti(d, dsMF2R(ks)))
}
func (d rdsImpl) GetMulti(ks []gae.DSKey, dst interface{}) error {
	plss, err := multiWrap(dst)
	if err != nil {
		return err
	}
	return gae.FixError(datastore.GetMulti(d, dsMF2R(ks), plss))
}
func (d rdsImpl) PutMulti(key []gae.DSKey, src interface{}) ([]gae.DSKey, error) {
	plss, err := multiWrap(src)
	if err != nil {
		return nil, err
	}
	ks, err := datastore.PutMulti(d, dsMF2R(key), plss)
	return dsMR2F(ks), gae.FixError(err)
}

// DSQueryer
func (d rdsImpl) NewQuery(kind string) gae.DSQuery {
	return queryImpl{datastore.NewQuery(kind)}
}
func (d rdsImpl) Run(q gae.DSQuery) gae.DSIterator {
	return iteratorImpl{q.(queryImpl).Query.Run(d)}
}
func (d rdsImpl) Count(q gae.DSQuery) (int, error) {
	return q.(queryImpl).Query.Count(d)
}
func (d rdsImpl) GetAll(q gae.DSQuery, dst interface{}) ([]gae.DSKey, error) {
	ks, err := q.(queryImpl).GetAll(d, dst)
	return dsMR2F(ks), err
}

// Transactioner
func (d rdsImpl) RunInTransaction(f func(c context.Context) error, opts *gae.DSTransactionOptions) error {
	ropts := (*datastore.TransactionOptions)(opts)
	return datastore.RunInTransaction(d, f, ropts)
}
