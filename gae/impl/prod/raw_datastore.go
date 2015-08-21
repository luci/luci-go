// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

// useRDS adds a gae.RawDatastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return ds.SetRawFactory(c, func(ci context.Context) ds.RawInterface {
		return rdsImpl{ci, info.Get(ci).GetNamespace()}
	})
}

////////// Query

type queryImpl struct{ *datastore.Query }

func (q queryImpl) Distinct() ds.Query {
	return queryImpl{q.Query.Distinct()}
}
func (q queryImpl) End(c ds.Cursor) ds.Query {
	return queryImpl{q.Query.End(c.(datastore.Cursor))}
}
func (q queryImpl) EventualConsistency() ds.Query {
	return queryImpl{q.Query.EventualConsistency()}
}
func (q queryImpl) KeysOnly() ds.Query {
	return queryImpl{q.Query.KeysOnly()}
}
func (q queryImpl) Limit(limit int) ds.Query {
	return queryImpl{q.Query.Limit(limit)}
}
func (q queryImpl) Offset(offset int) ds.Query {
	return queryImpl{q.Query.Offset(offset)}
}
func (q queryImpl) Order(fieldName string) ds.Query {
	return queryImpl{q.Query.Order(fieldName)}
}
func (q queryImpl) Start(c ds.Cursor) ds.Query {
	return queryImpl{q.Query.Start(c.(datastore.Cursor))}
}
func (q queryImpl) Ancestor(ancestor ds.Key) ds.Query {
	return queryImpl{q.Query.Ancestor(dsF2R(ancestor))}
}
func (q queryImpl) Project(fieldNames ...string) ds.Query {
	return queryImpl{q.Query.Project(fieldNames...)}
}
func (q queryImpl) Filter(filterStr string, value interface{}) ds.Query {
	return queryImpl{q.Query.Filter(filterStr, value)}
}

////////// Datastore

type rdsImpl struct {
	context.Context

	ns string
}

func (d rdsImpl) NewKey(kind, stringID string, intID int64, parent ds.Key) ds.Key {
	return dsR2F(datastore.NewKey(d, kind, stringID, intID, dsF2R(parent)))
}

func (rdsImpl) DecodeKey(encoded string) (ds.Key, error) {
	k, err := datastore.DecodeKey(encoded)
	return dsR2F(k), err
}

func idxCallbacker(err error, amt int, cb func(idx int, err error)) error {
	if err == nil {
		for i := 0; i < amt; i++ {
			cb(i, nil)
		}
		return nil
	}
	me, ok := err.(appengine.MultiError)
	if ok {
		for i, err := range me {
			cb(i, err)
		}
		return nil
	}
	return err
}

func (d rdsImpl) DeleteMulti(ks []ds.Key, cb ds.DeleteMultiCB) error {
	err := datastore.DeleteMulti(d, dsMF2R(ks))
	return idxCallbacker(err, len(ks), func(_ int, err error) {
		cb(err)
	})
}

func (d rdsImpl) GetMulti(keys []ds.Key, _meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	rkeys := dsMF2R(keys)
	vals := make([]datastore.PropertyLoadSaver, len(keys))
	for i := range keys {
		vals[i] = &typeFilter{ds.PropertyMap{}}
	}
	err := datastore.GetMulti(d, rkeys, vals)
	return idxCallbacker(err, len(keys), func(idx int, err error) {
		cb(vals[idx].(*typeFilter).pm, err)
	})
}

func (d rdsImpl) PutMulti(keys []ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) error {
	rkeys := dsMF2R(keys)
	rvals := make([]datastore.PropertyLoadSaver, len(vals))
	for i, val := range vals {
		rvals[i] = &typeFilter{val}
	}
	rkeys, err := datastore.PutMulti(d, rkeys, rvals)
	return idxCallbacker(err, len(keys), func(idx int, err error) {
		k := ds.Key(nil)
		if err == nil {
			k = dsR2F(rkeys[idx])
		}
		cb(k, err)
	})
}

func (d rdsImpl) NewQuery(kind string) ds.Query {
	return queryImpl{datastore.NewQuery(kind)}
}

func (d rdsImpl) DecodeCursor(s string) (ds.Cursor, error) {
	return datastore.DecodeCursor(s)
}

func (d rdsImpl) Run(q ds.Query, cb ds.RawRunCB) error {
	tf := typeFilter{}
	t := q.(queryImpl).Query.Run(d)
	cfunc := func() (ds.Cursor, error) {
		return t.Cursor()
	}
	for {
		k, err := t.Next(&tf)
		if err == datastore.Done {
			return nil
		}
		if err != nil {
			return err
		}
		if !cb(dsR2F(k), tf.pm, cfunc) {
			return nil
		}
	}
}

func (d rdsImpl) RunInTransaction(f func(c context.Context) error, opts *ds.TransactionOptions) error {
	ropts := (*datastore.TransactionOptions)(opts)
	return datastore.RunInTransaction(d, f, ropts)
}

func (d rdsImpl) Testable() ds.Testable {
	return nil
}
