// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prod

import (
	"go.chromium.org/gae/impl/prod/constraints"
	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

// useRDS adds a gae.RawDatastore implementation to context, accessible
// by gae.GetDS(c)
func useRDS(c context.Context) context.Context {
	return ds.SetRawFactory(c, func(ci context.Context) ds.RawInterface {
		rds := rdsImpl{
			userCtx: ci,
			ps:      getProdState(ci),
		}
		rds.aeCtx = rds.ps.context(ci)
		return &rds
	})
}

////////// Datastore

type rdsImpl struct {
	// userCtx is the context that has the luci/gae services and user objects in
	// it.
	userCtx context.Context

	// aeCtx is the AppEngine Context that will be used in method calls. This is
	// derived from ps.
	aeCtx context.Context

	// ps is the current production state.
	ps prodState
}

func fixMultiError(err error) error {
	if err == nil {
		return nil
	}
	if baseME, ok := err.(appengine.MultiError); ok {
		return errors.NewMultiError(baseME...)
	}
	return err
}

func idxCallbacker(err error, amt int, cb func(idx int, err error) error) error {
	if err == nil {
		for i := 0; i < amt; i++ {
			if err := cb(i, nil); err != nil {
				return err
			}
		}
		return nil
	}
	err = fixMultiError(err)
	me, ok := err.(errors.MultiError)
	if ok {
		for i, err := range me {
			if err := cb(i, err); err != nil {
				return err
			}
		}
		return nil
	}
	return err
}

func (d *rdsImpl) AllocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	// Map keys by entity type.
	entityMap := make(map[string][]int)
	for i, key := range keys {
		ks := key.String()
		entityMap[ks] = append(entityMap[ks], i)
	}

	// Allocate a set of IDs for each unique entity type.
	errors := errors.NewLazyMultiError(len(keys))
	setErrs := func(idxs []int, err error) {
		for _, idx := range idxs {
			errors.Assign(idx, err)
		}
	}

	for _, idxs := range entityMap {
		incomplete := keys[idxs[0]]
		par, err := dsF2R(d.aeCtx, incomplete.Parent())
		if err != nil {
			setErrs(idxs, err)
			continue
		}

		start, _, err := datastore.AllocateIDs(d.aeCtx, incomplete.Kind(), par, len(idxs))
		if err != nil {
			setErrs(idxs, err)
			continue
		}

		for i, idx := range idxs {
			keys[idx] = incomplete.WithID("", start+int64(i))
		}
	}

	for i, key := range keys {
		if err := errors.GetOne(i); err != nil {
			cb(i, nil, err)
		} else {
			cb(i, key, nil)
		}
	}
	return nil
}

func (d *rdsImpl) DeleteMulti(ks []*ds.Key, cb ds.DeleteMultiCB) error {
	keys, err := dsMF2R(d.aeCtx, ks)
	if err == nil {
		err = datastore.DeleteMulti(d.aeCtx, keys)
	}
	return idxCallbacker(err, len(ks), func(idx int, err error) error {
		return cb(idx, err)
	})
}

func (d *rdsImpl) GetMulti(keys []*ds.Key, _meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	vals := make([]datastore.PropertyLoadSaver, len(keys))
	rkeys, err := dsMF2R(d.aeCtx, keys)
	if err == nil {
		for i := range keys {
			vals[i] = &typeFilter{d.aeCtx, ds.PropertyMap{}}
		}
		err = datastore.GetMulti(d.aeCtx, rkeys, vals)
	}
	return idxCallbacker(err, len(keys), func(idx int, err error) error {
		if pls := vals[idx]; pls != nil {
			return cb(idx, pls.(*typeFilter).pm, err)
		}
		return cb(idx, nil, err)
	})
}

func (d *rdsImpl) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	rkeys, err := dsMF2R(d.aeCtx, keys)
	if err == nil {
		rvals := make([]datastore.PropertyLoadSaver, len(vals))
		for i, val := range vals {
			rvals[i] = &typeFilter{d.aeCtx, val}
		}
		rkeys, err = datastore.PutMulti(d.aeCtx, rkeys, rvals)
	}
	return idxCallbacker(err, len(keys), func(idx int, err error) error {
		k := (*ds.Key)(nil)
		if err == nil {
			k = dsR2F(rkeys[idx])
		}
		return cb(idx, k, err)
	})
}

func (d *rdsImpl) fixQuery(fq *ds.FinalizedQuery) (*datastore.Query, error) {
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
			p, err := dsF2RProp(d.aeCtx, vals[0])
			if err != nil {
				return nil, err
			}
			ret = ret.Ancestor(p.Value.(*datastore.Key))
		} else {
			filt := prop + "="
			for _, v := range vals {
				p, err := dsF2RProp(d.aeCtx, v)
				if err != nil {
					return nil, err
				}

				// Filter doesn't like ByteString, even though ByteString is the indexed
				// counterpart of []byte... sigh.
				if ds, ok := p.Value.(datastore.ByteString); ok {
					p.Value = []byte(ds)
				}
				ret = ret.Filter(filt, p.Value)
			}
		}
	}

	if lnam, lop, lprop := fq.IneqFilterLow(); lnam != "" {
		p, err := dsF2RProp(d.aeCtx, lprop)
		if err != nil {
			return nil, err
		}
		ret = ret.Filter(lnam+" "+lop, p.Value)
	}

	if hnam, hop, hprop := fq.IneqFilterHigh(); hnam != "" {
		p, err := dsF2RProp(d.aeCtx, hprop)
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

func (d *rdsImpl) DecodeCursor(s string) (ds.Cursor, error) {
	return datastore.DecodeCursor(s)
}

func (d *rdsImpl) Run(fq *ds.FinalizedQuery, cb ds.RawRunCB) error {
	q, err := d.fixQuery(fq)
	if err != nil {
		return err
	}

	t := q.Run(d.aeCtx)

	cfunc := func() (ds.Cursor, error) {
		return t.Cursor()
	}
	tf := typeFilter{}
	for {
		k, err := t.Next(&tf)
		if err == datastore.Done {
			return nil
		}
		if err != nil {
			return err
		}
		if err := cb(dsR2F(k), tf.pm, cfunc); err != nil {
			return err
		}
	}
}

func (d *rdsImpl) Count(fq *ds.FinalizedQuery) (int64, error) {
	q, err := d.fixQuery(fq)
	if err != nil {
		return 0, err
	}
	ret, err := q.Count(d.aeCtx)
	return int64(ret), err
}

func (d *rdsImpl) RunInTransaction(f func(c context.Context) error, opts *ds.TransactionOptions) error {
	ropts := (*datastore.TransactionOptions)(opts)
	return datastore.RunInTransaction(d.aeCtx, func(c context.Context) error {
		// Derive a prodState with this transaction Context.
		ps := d.ps
		ps.ctx = c
		ps.inTxn = true

		c = withProdState(d.userCtx, ps)
		return f(c)
	}, ropts)
}

func (d *rdsImpl) WithoutTransaction() context.Context {
	c := d.userCtx
	if d.ps.inTxn {
		// We're in a transaction. Reset to non-transactional state.
		ps := d.ps
		ps.ctx = ps.noTxnCtx
		ps.inTxn = false
		c = withProdState(c, ps)
	}
	return c
}

func (d *rdsImpl) CurrentTransaction() ds.Transaction {
	if d.ps.inTxn {
		// Since we don't distinguish between transactions (yet), we just need this
		// to be non-nil.
		return struct{}{}
	}
	return nil
}

func (d *rdsImpl) Constraints() ds.Constraints { return constraints.DS() }

func (d *rdsImpl) GetTestable() ds.Testable {
	return nil
}
