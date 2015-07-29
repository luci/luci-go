// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	rds "github.com/luci/gae/service/rawdatastore"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

// useRDS adds a gae.RawDatastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return rds.SetFactory(c, func(ci context.Context) rds.Interface {
		// TODO(riannucci): Track namespace in a better way
		k := datastore.NewKey(ci, "kind", "", 1, nil) // get current namespace.
		return rdsImpl{ci, k.Namespace()}
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

////////// Datastore

type rdsImpl struct {
	context.Context

	ns string
}

func (d rdsImpl) NewKey(kind, stringID string, intID int64, parent rds.Key) rds.Key {
	return dsR2F(datastore.NewKey(d, kind, stringID, intID, dsF2R(parent)))
}

func (rdsImpl) DecodeKey(encoded string) (rds.Key, error) {
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

func (d rdsImpl) DeleteMulti(ks []rds.Key, cb rds.DeleteMultiCB) error {
	err := datastore.DeleteMulti(d, dsMF2R(ks))
	return idxCallbacker(err, len(ks), func(_ int, err error) {
		cb(err)
	})
}

func (d rdsImpl) GetMulti(keys []rds.Key, cb rds.GetMultiCB) error {
	rkeys := dsMF2R(keys)
	vals := make([]datastore.PropertyLoadSaver, len(keys))
	for i := range keys {
		vals[i] = &typeFilter{rds.PropertyMap{}}
	}
	err := datastore.GetMulti(d, rkeys, vals)
	return idxCallbacker(err, len(keys), func(idx int, err error) {
		cb(vals[idx].(*typeFilter).pm, err)
	})
}

func (d rdsImpl) PutMulti(keys []rds.Key, vals []rds.PropertyLoadSaver, cb rds.PutMultiCB) error {
	rkeys := dsMF2R(keys)
	rvals := make([]datastore.PropertyLoadSaver, len(vals))
	for i, val := range vals {
		rvals[i] = &typeFilter{val.(rds.PropertyMap)}
	}
	rkeys, err := datastore.PutMulti(d, rkeys, vals)
	return idxCallbacker(err, len(keys), func(idx int, err error) {
		k := rds.Key(nil)
		if err == nil {
			k = dsR2F(rkeys[idx])
		}
		cb(k, err)
	})
}

func (d rdsImpl) NewQuery(kind string) rds.Query {
	return queryImpl{datastore.NewQuery(kind)}
}

func (d rdsImpl) Run(q rds.Query, cb rds.RunCB) error {
	tf := typeFilter{}
	t := q.(queryImpl).Query.Run(d)
	cfunc := func() (rds.Cursor, error) {
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

func (d rdsImpl) RunInTransaction(f func(c context.Context) error, opts *rds.TransactionOptions) error {
	ropts := (*datastore.TransactionOptions)(opts)
	return datastore.RunInTransaction(d, f, ropts)
}
