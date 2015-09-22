// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

// useRDS adds a gae.RawDatastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return ds.SetRawFactory(c, func(ci context.Context) ds.RawInterface {
		return rdsImpl{ci, info.Get(ci).GetNamespace()}
	})
}

////////// Datastore

type rdsImpl struct {
	context.Context

	ns string
}

func idxCallbacker(err error, amt int, cb func(idx int, err error)) error {
	if err == nil {
		for i := 0; i < amt; i++ {
			cb(i, nil)
		}
		return nil
	}
	err = errors.Fix(err)
	me, ok := err.(errors.MultiError)
	if ok {
		for i, err := range me {
			cb(i, err)
		}
		return nil
	}
	return err
}

func (d rdsImpl) AllocateIDs(incomplete *ds.Key, n int) (start int64, err error) {
	par, err := dsF2R(d, incomplete.Parent())
	if err != nil {
		return
	}

	start, _, err = datastore.AllocateIDs(d, incomplete.Last().Kind, par, n)
	return
}

func (d rdsImpl) DeleteMulti(ks []*ds.Key, cb ds.DeleteMultiCB) error {
	keys, err := dsMF2R(d, ks)
	if err == nil {
		err = datastore.DeleteMulti(d, keys)
	}
	return idxCallbacker(err, len(ks), func(_ int, err error) {
		cb(err)
	})
}

func (d rdsImpl) GetMulti(keys []*ds.Key, _meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	vals := make([]datastore.PropertyLoadSaver, len(keys))
	rkeys, err := dsMF2R(d, keys)
	if err == nil {
		for i := range keys {
			vals[i] = &typeFilter{d, ds.PropertyMap{}}
		}
		err = datastore.GetMulti(d, rkeys, vals)
	}
	return idxCallbacker(err, len(keys), func(idx int, err error) {
		if pls := vals[idx]; pls != nil {
			cb(pls.(*typeFilter).pm, err)
		} else {
			cb(nil, err)
		}
	})
}

func (d rdsImpl) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) error {
	rkeys, err := dsMF2R(d, keys)
	if err == nil {
		rvals := make([]datastore.PropertyLoadSaver, len(vals))
		for i, val := range vals {
			rvals[i] = &typeFilter{d, val}
		}
		rkeys, err = datastore.PutMulti(d, rkeys, rvals)
	}
	return idxCallbacker(err, len(keys), func(idx int, err error) {
		k := (*ds.Key)(nil)
		if err == nil {
			k = dsR2F(rkeys[idx])
		}
		cb(k, err)
	})
}

func (d rdsImpl) fixQuery(fq *ds.FinalizedQuery) (*datastore.Query, error) {
	ret := datastore.NewQuery(fq.Kind())

	start, end := fq.Bounds()
	if start != nil {
		ret = ret.Start(start.(datastore.Cursor))
	}
	if end != nil {
		ret = ret.End(end.(datastore.Cursor))
	}

	for prop, vals := range fq.EqFilters() {
		if prop == "__ancestor__" {
			p, err := dsF2RProp(d, vals[0])
			if err != nil {
				return nil, err
			}
			ret = ret.Ancestor(p.Value.(*datastore.Key))
		} else {
			filt := prop + "="
			for _, v := range vals {
				p, err := dsF2RProp(d, v)
				if err != nil {
					return nil, err
				}

				ret = ret.Filter(filt, p.Value)
			}
		}
	}

	if lnam, lop, lprop := fq.IneqFilterLow(); lnam != "" {
		p, err := dsF2RProp(d, lprop)
		if err != nil {
			return nil, err
		}
		ret = ret.Filter(lnam+" "+lop, p.Value)
	}

	if hnam, hop, hprop := fq.IneqFilterHigh(); hnam != "" {
		p, err := dsF2RProp(d, hprop)
		if err != nil {
			return nil, err
		}
		ret = ret.Filter(hnam+" "+hop, p.Value)
	}

	if fq.EventuallyConsistent() {
		ret = ret.EventualConsistency()
	}

	if fq.KeysOnly() {
		ret = ret.KeysOnly()
	}

	if lim, ok := fq.Limit(); ok {
		ret = ret.Limit(int(lim))
	}

	if off, ok := fq.Offset(); ok {
		ret = ret.Offset(int(off))
	}

	for _, o := range fq.Orders() {
		ret = ret.Order(o.String())
	}

	ret = ret.Project(fq.Project()...)
	if fq.Distinct() {
		ret = ret.Distinct()
	}

	return ret, nil
}

func (d rdsImpl) DecodeCursor(s string) (ds.Cursor, error) {
	return datastore.DecodeCursor(s)
}

func (d rdsImpl) Run(fq *ds.FinalizedQuery, cb ds.RawRunCB) error {
	tf := typeFilter{}
	q, err := d.fixQuery(fq)
	if err != nil {
		return err
	}

	t := q.Run(d)

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
