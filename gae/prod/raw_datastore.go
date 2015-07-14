// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"golang.org/x/net/context"

	"infra/gae/libs/gae"

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

var _ gae.RDSIterator = iteratorImpl{}

func (i iteratorImpl) Cursor() (gae.DSCursor, error) {
	return i.Iterator.Cursor()
}

func (i iteratorImpl) Next(pls gae.DSPropertyLoadSaver) (gae.DSKey, error) {
	return dsR2FErr(i.Iterator.Next(&typeFilter{pls}))
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

func multiWrap(os []gae.DSPropertyLoadSaver) []datastore.PropertyLoadSaver {
	ret := make([]datastore.PropertyLoadSaver, len(os))
	for i, pls := range os {
		ret[i] = &typeFilter{pls}
	}
	return ret
}

func (d rdsImpl) Delete(k gae.DSKey) error { return datastore.Delete(d, dsF2R(k)) }
func (d rdsImpl) Get(key gae.DSKey, dst gae.DSPropertyLoadSaver) error {
	return datastore.Get(d, dsF2R(key), &typeFilter{dst})
}
func (d rdsImpl) Put(key gae.DSKey, src gae.DSPropertyLoadSaver) (gae.DSKey, error) {
	return dsR2FErr(datastore.Put(d, dsF2R(key), &typeFilter{src}))
}

func (d rdsImpl) DeleteMulti(ks []gae.DSKey) error {
	return gae.FixError(datastore.DeleteMulti(d, dsMF2R(ks)))
}

func (d rdsImpl) GetMulti(ks []gae.DSKey, plss []gae.DSPropertyLoadSaver) error {
	return gae.FixError(datastore.GetMulti(d, dsMF2R(ks), multiWrap(plss)))
}
func (d rdsImpl) PutMulti(key []gae.DSKey, plss []gae.DSPropertyLoadSaver) ([]gae.DSKey, error) {
	ks, err := datastore.PutMulti(d, dsMF2R(key), multiWrap(plss))
	return dsMR2F(ks), gae.FixError(err)
}

// DSQueryer
func (d rdsImpl) NewQuery(kind string) gae.DSQuery {
	return queryImpl{datastore.NewQuery(kind)}
}
func (d rdsImpl) Run(q gae.DSQuery) gae.RDSIterator {
	return iteratorImpl{q.(queryImpl).Query.Run(d)}
}
func (d rdsImpl) Count(q gae.DSQuery) (int, error) {
	return q.(queryImpl).Query.Count(d)
}
func (d rdsImpl) GetAll(q gae.DSQuery, dst *[]gae.DSPropertyMap) ([]gae.DSKey, error) {
	fakeDst := []datastore.PropertyList(nil)
	ks, err := q.(queryImpl).GetAll(d, &fakeDst)
	if err != nil {
		return nil, err
	}
	*dst = make([]gae.DSPropertyMap, len(fakeDst))
	for i, pl := range fakeDst {
		(*dst)[i] = gae.DSPropertyMap{}
		if err := (&typeFilter{(*dst)[i]}).Load(pl); err != nil {
			return nil, err
		}
	}
	return dsMR2F(ks), err
}

// Transactioner
func (d rdsImpl) RunInTransaction(f func(c context.Context) error, opts *gae.DSTransactionOptions) error {
	ropts := (*datastore.TransactionOptions)(opts)
	return datastore.RunInTransaction(d, f, ropts)
}
