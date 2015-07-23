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

func (r *rdsState) GetAll(q rds.Query, dst *[]rds.PropertyMap) (ret []rds.Key, err error) {
	err = r.run(func() (err error) {
		ret, err = r.Interface.GetAll(q, dst)
		return
	})
	return
}

func (r *rdsState) Count(q rds.Query) (ret int, err error) {
	err = r.run(func() (err error) {
		ret, err = r.Interface.Count(q)
		return
	})
	return
}

func (r *rdsState) RunInTransaction(f func(c context.Context) error, opts *rds.TransactionOptions) error {
	return r.run(func() error {
		return r.Interface.RunInTransaction(f, opts)
	})
}

func (r *rdsState) Put(key rds.Key, src rds.PropertyLoadSaver) (ret rds.Key, err error) {
	err = r.run(func() (err error) {
		ret, err = r.Interface.Put(key, src)
		return
	})
	return
}

func (r *rdsState) Get(key rds.Key, dst rds.PropertyLoadSaver) error {
	return r.run(func() error {
		return r.Interface.Get(key, dst)
	})
}

func (r *rdsState) Delete(key rds.Key) error {
	return r.run(func() error {
		return r.Interface.Delete(key)
	})
}

func (r *rdsState) DeleteMulti(keys []rds.Key) error {
	return r.run(func() error {
		return r.Interface.DeleteMulti(keys)
	})
}

func (r *rdsState) GetMulti(keys []rds.Key, dst []rds.PropertyLoadSaver) error {
	return r.run(func() error {
		return r.Interface.GetMulti(keys, dst)
	})
}

func (r *rdsState) PutMulti(keys []rds.Key, src []rds.PropertyLoadSaver) (ret []rds.Key, err error) {
	err = r.run(func() (err error) {
		ret, err = r.Interface.PutMulti(keys, src)
		return
	})
	return
}

// FilterRDS installs a counter RawDatastore filter in the context.
func FilterRDS(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return rds.AddFilters(c, func(ic context.Context, RawDatastore rds.Interface) rds.Interface {
		return &rdsState{state, RawDatastore}
	}), state
}
