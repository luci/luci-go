// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
)

type dsState struct {
	*state

	ds.Interface
}

func (r *dsState) DecodeKey(encoded string) (ret ds.Key, err error) {
	err = r.run(func() (err error) {
		ret, err = r.Interface.DecodeKey(encoded)
		return
	})
	return
}

func (r *dsState) RunInTransaction(f func(c context.Context) error, opts *ds.TransactionOptions) error {
	return r.run(func() error {
		return r.Interface.RunInTransaction(f, opts)
	})
}

// TODO(riannucci): Allow the user to specify a multierror which will propagate
// to the callback correctly.

func (r *dsState) DeleteMulti(keys []ds.Key, cb ds.DeleteMultiCB) error {
	return r.run(func() error {
		return r.Interface.DeleteMulti(keys, cb)
	})
}

func (r *dsState) GetMulti(keys []ds.Key, cb ds.GetMultiCB) error {
	return r.run(func() error {
		return r.Interface.GetMulti(keys, cb)
	})
}

func (r *dsState) PutMulti(keys []ds.Key, vals []ds.PropertyLoadSaver, cb ds.PutMultiCB) error {
	return r.run(func() (err error) {
		return r.Interface.PutMulti(keys, vals, cb)
	})
}

// FilterRDS installs a counter datastore filter in the context.
func FilterRDS(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return ds.AddFilters(c, func(ic context.Context, datastore ds.Interface) ds.Interface {
		return &dsState{state, datastore}
	}), state
}
