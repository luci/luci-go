// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dscache

import (
	ds "github.com/luci/gae/service/datastore"
)

type dsTxnCache struct {
	ds.RawInterface

	state *dsTxnState

	sc *supportContext
}

var _ ds.RawInterface = (*dsTxnCache)(nil)

func (d *dsTxnCache) DeleteMulti(keys []ds.Key, cb ds.DeleteMultiCB) error {
	d.state.add(d.sc, keys)
	return d.RawInterface.DeleteMulti(keys, cb)
}

func (d *dsTxnCache) PutMulti(keys []ds.Key, metas []ds.PropertyMap, cb ds.PutMultiCB) error {
	d.state.add(d.sc, keys)
	return d.RawInterface.PutMulti(keys, metas, cb)
}

// TODO(riannucci): on GetAll, Load from memcache and invalidate entries if the
// memcache version doesn't match the datastore version.
