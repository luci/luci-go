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
}

func (r *readOnlyDatastore) AllocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	return ErrReadOnly
}

func (r *readOnlyDatastore) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	return ErrReadOnly
}

func (r *readOnlyDatastore) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	return ErrReadOnly
}

// FilterRDS installs a read-only datastore filter in the context.
//
// This enforces that all datastore operations which could mutate the datastore
// will return ErrReadOnly.
func FilterRDS(c context.Context) context.Context {
	return ds.AddRawFilters(c, func(ic context.Context, inner ds.RawInterface) ds.RawInterface {
		return &readOnlyDatastore{inner}
	})
}
