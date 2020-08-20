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

package dscache

import (
	ds "go.chromium.org/gae/service/datastore"
)

type dsTxnCache struct {
	ds.RawInterface

	state *dsTxnState

	sc *supportContext
}

var _ ds.RawInterface = (*dsTxnCache)(nil)

func (d *dsTxnCache) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	d.state.add(d.sc, keys)
	return d.RawInterface.DeleteMulti(keys, cb)
}

func (d *dsTxnCache) PutMulti(keys []*ds.Key, metas []ds.PropertyMap, cb ds.NewKeyCB) error {
	d.state.add(d.sc, keys)
	return d.RawInterface.PutMulti(keys, metas, cb)
}

// TODO(riannucci): on GetAll, Load from memcache and invalidate entries if the
// memcache version doesn't match the datastore version.
