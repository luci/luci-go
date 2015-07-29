// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"golang.org/x/net/context"

	rds "github.com/luci/gae/service/rawdatastore"
)

type rdsState struct {
	*state

	rds.Interface
}

func (r *rdsState) DecodeKey(encoded string) (ret rds.Key, err error) {
	err = r.run(func() (err error) {
		ret, err = r.Interface.DecodeKey(encoded)
		return
	})
	return
}

func (r *rdsState) RunInTransaction(f func(c context.Context) error, opts *rds.TransactionOptions) error {
	return r.run(func() error {
		return r.Interface.RunInTransaction(f, opts)
	})
}

// TODO(riannucci): Allow the user to specify a multierror which will propagate
// to the callback correctly.

func (r *rdsState) DeleteMulti(keys []rds.Key, cb rds.DeleteMultiCB) error {
	return r.run(func() error {
		return r.Interface.DeleteMulti(keys, cb)
	})
}

func (r *rdsState) GetMulti(keys []rds.Key, cb rds.GetMultiCB) error {
	return r.run(func() error {
		return r.Interface.GetMulti(keys, cb)
	})
}

func (r *rdsState) PutMulti(keys []rds.Key, vals []rds.PropertyLoadSaver, cb rds.PutMultiCB) error {
	return r.run(func() (err error) {
		return r.Interface.PutMulti(keys, vals, cb)
	})
}

// FilterRDS installs a counter RawDatastore filter in the context.
func FilterRDS(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return rds.AddFilters(c, func(ic context.Context, RawDatastore rds.Interface) rds.Interface {
		return &rdsState{state, RawDatastore}
	}), state
}
