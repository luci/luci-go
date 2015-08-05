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

	ds.RawInterface
}

func (r *dsState) DecodeKey(encoded string) (ret ds.Key, err error) {
	err = r.run(func() (err error) {
		ret, err = r.RawInterface.DecodeKey(encoded)
		return
	})
	return
}

func (r *dsState) RunInTransaction(f func(c context.Context) error, opts *ds.TransactionOptions) error {
	return r.run(func() error {
		return r.RawInterface.RunInTransaction(f, opts)
	})
}

// TODO(riannucci): Allow the user to specify a multierror which will propagate
// to the callback correctly.

func (r *dsState) DeleteMulti(keys []ds.Key, cb ds.DeleteMultiCB) error {
	return r.run(func() error {
		return r.RawInterface.DeleteMulti(keys, cb)
	})
}

func (r *dsState) GetMulti(keys []ds.Key, meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	return r.run(func() error {
		return r.RawInterface.GetMulti(keys, meta, cb)
	})
}

func (r *dsState) PutMulti(keys []ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) error {
	return r.run(func() (err error) {
		return r.RawInterface.PutMulti(keys, vals, cb)
	})
}

// FilterRDS installs a counter datastore filter in the context.
func FilterRDS(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return ds.AddRawFilters(c, func(ic context.Context, RawDatastore ds.RawInterface) ds.RawInterface {
		return &dsState{state, RawDatastore}
	}), state
}
