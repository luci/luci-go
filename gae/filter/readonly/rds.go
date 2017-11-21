// Copyright 2017 The LUCI Authors.
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

// Package readonly implements a filter that enforces read-only accesses to
// datastore.
//
// This is useful in hybrid environments where one cluster wants to read from
// a cache-backed datastore, but cannot modify the cache, so reads are safe and
// direct, but writes would create a state where the cached values are invalid.
// This happens when mixing AppEngine datastore/memcache with Cloud Datastore
// readers.
package readonly

import (
	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
)

// ErrReadOnly is an error returned in response to mutating datastore
// operations.
var ErrReadOnly = errors.New("readonly: datastore is read-only")

// readOnlyDatastore is a datastore.RawInterface implementation that returns
// ErrReadOnly on mutating operations.
type readOnlyDatastore struct {
	ds.RawInterface
	isRO Predicate
}

type perKeyCB func(idx int, err error) error
type implCB func(mutable []int, cb perKeyCB) error

func (r *readOnlyDatastore) run(keys []*ds.Key, impl implCB, cb perKeyCB) error {
	var mutable []int
	if r.isRO != nil { // all keys are RO by default
		for i, k := range keys {
			if !r.isRO(k) {
				mutable = append(mutable, i)
			}
		}
	}

	if len(mutable) != 0 {
		if err := impl(mutable, cb); err != nil {
			return err
		}
	}

	cur := 0 // cursor inside 'mutable' array
	for idx := 0; idx < len(keys); idx++ {
		if cur != len(mutable) && idx == mutable[cur] {
			cur++
			continue // results for 'idx' was already delivered above
		}
		if err := cb(idx, ErrReadOnly); err != nil {
			return err
		}
	}

	return nil
}

func (r *readOnlyDatastore) AllocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	impl := func(mutable []int, cb perKeyCB) error {
		mutableKeys := make([]*ds.Key, len(mutable))
		for i, idx := range mutable {
			mutableKeys[i] = keys[idx]
		}
		return r.RawInterface.AllocateIDs(mutableKeys, func(idx int, key *ds.Key, err error) error {
			return cb(mutable[idx], err)
		})
	}
	return r.run(keys, impl, func(idx int, err error) error {
		return cb(idx, keys[idx], err)
	})
}

func (r *readOnlyDatastore) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	impl := func(mutable []int, cb perKeyCB) error {
		mutableKeys := make([]*ds.Key, len(mutable))
		for i, idx := range mutable {
			mutableKeys[i] = keys[idx]
		}
		return r.RawInterface.DeleteMulti(mutableKeys, func(idx int, err error) error {
			return cb(mutable[idx], err)
		})
	}
	return r.run(keys, impl, perKeyCB(cb))
}

func (r *readOnlyDatastore) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	impl := func(mutable []int, cb perKeyCB) error {
		mutableKeys := make([]*ds.Key, len(mutable))
		mutableVals := make([]ds.PropertyMap, len(mutable))
		for i, idx := range mutable {
			mutableKeys[i] = keys[idx]
			mutableVals[i] = vals[idx]
		}
		return r.RawInterface.PutMulti(mutableKeys, mutableVals, func(idx int, key *ds.Key, err error) error {
			return cb(mutable[idx], err)
		})
	}
	return r.run(keys, impl, func(idx int, err error) error {
		return cb(idx, keys[idx], err)
	})
}

// Predicate is a user-supplied function that examines a key and returns true if
// it should be treated as read-only.
type Predicate func(*ds.Key) (isReadOnly bool)

// FilterRDS installs a read-only datastore filter in the context.
//
// This enforces that mutating datastore operations that touch keys for which
// the predicate returns 'true' fail with ErrReadOnly.
//
// If the predicate is nil, all mutating operations are denied.
func FilterRDS(c context.Context, p Predicate) context.Context {
	return ds.AddRawFilters(c, func(ic context.Context, inner ds.RawInterface) ds.RawInterface {
		return &readOnlyDatastore{inner, p}
	})
}
