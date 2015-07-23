// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"github.com/luci/gae"
	rds "github.com/luci/gae/service/rawdatastore"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

// useRDS adds a gae.RawDatastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return rds.SetFactory(c, func(ci context.Context) rds.Interface {
		return rdsImpl{ci}
	})
}

////////// Query

type queryImpl struct{ *datastore.Query }

func (q queryImpl) Distinct() rds.Query {
	return queryImpl{q.Query.Distinct()}
}
func (q queryImpl) End(c rds.Cursor) rds.Query {
	return queryImpl{q.Query.End(c.(datastore.Cursor))}
}
func (q queryImpl) EventualConsistency() rds.Query {
	return queryImpl{q.Query.EventualConsistency()}
}
func (q queryImpl) KeysOnly() rds.Query {
	return queryImpl{q.Query.KeysOnly()}
}
func (q queryImpl) Limit(limit int) rds.Query {
	return queryImpl{q.Query.Limit(limit)}
}
func (q queryImpl) Offset(offset int) rds.Query {
	return queryImpl{q.Query.Offset(offset)}
}
func (q queryImpl) Order(fieldName string) rds.Query {
	return queryImpl{q.Query.Order(fieldName)}
}
func (q queryImpl) Start(c rds.Cursor) rds.Query {
	return queryImpl{q.Query.Start(c.(datastore.Cursor))}
}
func (q queryImpl) Ancestor(ancestor rds.Key) rds.Query {
	return queryImpl{q.Query.Ancestor(dsF2R(ancestor))}
}
func (q queryImpl) Project(fieldNames ...string) rds.Query {
	return queryImpl{q.Query.Project(fieldNames...)}
}
func (q queryImpl) Filter(filterStr string, value interface{}) rds.Query {
	return queryImpl{q.Query.Filter(filterStr, value)}
}

////////// Iterator

type iteratorImpl struct{ *datastore.Iterator }

var _ rds.Iterator = iteratorImpl{}

func (i iteratorImpl) Cursor() (rds.Cursor, error) {
	return i.Iterator.Cursor()
}

func (i iteratorImpl) Next(pls rds.PropertyLoadSaver) (rds.Key, error) {
	return dsR2FErr(i.Iterator.Next(&typeFilter{pls}))
}

////////// Datastore

type rdsImpl struct{ context.Context }

// NewKeyer
func (d rdsImpl) NewKey(kind, stringID string, intID int64, parent rds.Key) rds.Key {
	return dsR2F(datastore.NewKey(d, kind, stringID, intID, dsF2R(parent)))
}

func (rdsImpl) DecodeKey(encoded string) (rds.Key, error) {
	return dsR2FErr(datastore.DecodeKey(encoded))
}

func multiWrap(os []rds.PropertyLoadSaver) []datastore.PropertyLoadSaver {
	ret := make([]datastore.PropertyLoadSaver, len(os))
	for i, pls := range os {
		ret[i] = &typeFilter{pls}
	}
	return ret
}

func (d rdsImpl) Delete(k rds.Key) error { return datastore.Delete(d, dsF2R(k)) }
func (d rdsImpl) Get(key rds.Key, dst rds.PropertyLoadSaver) error {
	return datastore.Get(d, dsF2R(key), &typeFilter{dst})
}
func (d rdsImpl) Put(key rds.Key, src rds.PropertyLoadSaver) (rds.Key, error) {
	return dsR2FErr(datastore.Put(d, dsF2R(key), &typeFilter{src}))
}

func (d rdsImpl) DeleteMulti(ks []rds.Key) error {
	return gae.FixError(datastore.DeleteMulti(d, dsMF2R(ks)))
}

func (d rdsImpl) GetMulti(ks []rds.Key, plss []rds.PropertyLoadSaver) error {
	return gae.FixError(datastore.GetMulti(d, dsMF2R(ks), multiWrap(plss)))
}
func (d rdsImpl) PutMulti(key []rds.Key, plss []rds.PropertyLoadSaver) ([]rds.Key, error) {
	ks, err := datastore.PutMulti(d, dsMF2R(key), multiWrap(plss))
	return dsMR2F(ks), gae.FixError(err)
}

// DSQueryer
func (d rdsImpl) NewQuery(kind string) rds.Query {
	return queryImpl{datastore.NewQuery(kind)}
}
func (d rdsImpl) Run(q rds.Query) rds.Iterator {
	return iteratorImpl{q.(queryImpl).Query.Run(d)}
}
func (d rdsImpl) Count(q rds.Query) (int, error) {
	return q.(queryImpl).Query.Count(d)
}
func (d rdsImpl) GetAll(q rds.Query, dst *[]rds.PropertyMap) ([]rds.Key, error) {
	fakeDst := []datastore.PropertyList(nil)
	ks, err := q.(queryImpl).GetAll(d, &fakeDst)
	if err != nil {
		return nil, err
	}
	*dst = make([]rds.PropertyMap, len(fakeDst))
	for i, pl := range fakeDst {
		(*dst)[i] = rds.PropertyMap{}
		if err := (&typeFilter{(*dst)[i]}).Load(pl); err != nil {
			return nil, err
		}
	}
	return dsMR2F(ks), err
}

// Transactioner
func (d rdsImpl) RunInTransaction(f func(c context.Context) error, opts *rds.TransactionOptions) error {
	ropts := (*datastore.TransactionOptions)(opts)
	return datastore.RunInTransaction(d, f, ropts)
}
