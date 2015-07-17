// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"golang.org/x/net/context"

	"infra/gae/libs/gae"
)

type rdsState struct {
	*state

	gae.RawDatastore
}

func (r *rdsState) DecodeKey(encoded string) (ret gae.DSKey, err error) {
	err = r.run(func() (err error) {
		ret, err = r.RawDatastore.DecodeKey(encoded)
		return
	})
	return
}

func (r *rdsState) GetAll(q gae.DSQuery, dst *[]gae.DSPropertyMap) (ret []gae.DSKey, err error) {
	err = r.run(func() (err error) {
		ret, err = r.RawDatastore.GetAll(q, dst)
		return
	})
	return
}

func (r *rdsState) Count(q gae.DSQuery) (ret int, err error) {
	err = r.run(func() (err error) {
		ret, err = r.RawDatastore.Count(q)
		return
	})
	return
}

func (r *rdsState) RunInTransaction(f func(c context.Context) error, opts *gae.DSTransactionOptions) error {
	return r.run(func() error {
		return r.RawDatastore.RunInTransaction(f, opts)
	})
}

func (r *rdsState) Put(key gae.DSKey, src gae.DSPropertyLoadSaver) (ret gae.DSKey, err error) {
	err = r.run(func() (err error) {
		ret, err = r.RawDatastore.Put(key, src)
		return
	})
	return
}

func (r *rdsState) Get(key gae.DSKey, dst gae.DSPropertyLoadSaver) error {
	return r.run(func() error {
		return r.RawDatastore.Get(key, dst)
	})
}

func (r *rdsState) Delete(key gae.DSKey) error {
	return r.run(func() error {
		return r.RawDatastore.Delete(key)
	})
}

func (r *rdsState) DeleteMulti(keys []gae.DSKey) error {
	return r.run(func() error {
		return r.RawDatastore.DeleteMulti(keys)
	})
}

func (r *rdsState) GetMulti(keys []gae.DSKey, dst []gae.DSPropertyLoadSaver) error {
	return r.run(func() error {
		return r.RawDatastore.GetMulti(keys, dst)
	})
}

func (r *rdsState) PutMulti(keys []gae.DSKey, src []gae.DSPropertyLoadSaver) (ret []gae.DSKey, err error) {
	err = r.run(func() (err error) {
		ret, err = r.RawDatastore.PutMulti(keys, src)
		return
	})
	return
}

// FilterRDS installs a counter RawDatastore filter in the context.
func FilterRDS(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return gae.AddRDSFilters(c, func(ic context.Context, RawDatastore gae.RawDatastore) gae.RawDatastore {
		return &rdsState{state, RawDatastore}
	}), state
}
