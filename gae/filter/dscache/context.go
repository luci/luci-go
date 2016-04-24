// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dscache

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	mc "github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/mathrand"
	"golang.org/x/net/context"
)

type key int

var dsTxnCacheKey key

// FilterRDS installs a caching RawDatastore filter in the context.
//
// It does nothing if IsGloballyEnabled returns false. That way it is possible
// to disable the cache in runtime (e.g. in case memcache service is having
// issues).
//
// shardsForKey is a user-controllable function which calculates the number of
// shards to use for a certain datastore key. The provided key will always be
// valid and complete.
//
// The # of shards returned may be between 1 and 256. Values above this range
// will be clamped into that range. A return value of 0 means that NO cache
// operations should be done for this key, regardless of the dscache.enable
// setting.
//
// If shardsForKey is nil, the value of DefaultShards is used for all keys.
func FilterRDS(c context.Context, shardsForKey func(*ds.Key) int) context.Context {
	if !IsGloballyEnabled(c) {
		return c
	}
	return AlwaysFilterRDS(c, shardsForKey)
}

// AlwaysFilterRDS installs a caching RawDatastore filter in the context.
//
// Unlike FilterRDS it doesn't check GlobalConfig via IsGloballyEnabled call,
// assuming caller already knows whether filter should be applied or not.
func AlwaysFilterRDS(c context.Context, shardsForKey func(*ds.Key) int) context.Context {
	return ds.AddRawFilters(c, func(c context.Context, ds ds.RawInterface) ds.RawInterface {
		i := info.Get(c)
		ns, _ := i.GetNamespace()

		sc := &supportContext{
			i.AppID(),
			ns,
			c,
			mc.Get(c),
			mathrand.Get(c),
			shardsForKey,
		}

		v := c.Value(dsTxnCacheKey)
		if v == nil {
			return &dsCache{ds, sc}
		}
		return &dsTxnCache{ds, v.(*dsTxnState), sc}
	})
}
