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
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/rand/mathrand"

	ds "go.chromium.org/gae/service/datastore"
)

var dsTxnCacheKey = "holds a *dsCache"
var dsShardFunctionsKey = "holds []ShardFunction"

// ShardFunction is a user-controllable function which calculates the number of
// shards to use for a certain datastore key. The provided key will always be
// valid and complete. It should return ok=true if it recognized the Key, and
// false otherwise.
//
// The # of shards returned may be between 1 and 256. Values above this range
// will be clamped into that range. A return value of 0 means that NO cache
// operations should be done for this key, regardless of the dscache.enable
// setting.
type ShardFunction func(*ds.Key) (shards int, ok bool)

// FilterRDS installs a caching RawDatastore filter in the context.
//
// It does nothing if IsGloballyEnabled returns false. That way it is possible
// to disable the cache in runtime (e.g. in case memcache service is having
// issues).
func FilterRDS(c context.Context) context.Context {
	if !IsGloballyEnabled(c) {
		return c
	}
	return AlwaysFilterRDS(c)
}

// AlwaysFilterRDS installs a caching RawDatastore filter in the context.
//
// Unlike FilterRDS it doesn't check GlobalConfig via IsGloballyEnabled call,
// assuming caller already knows whether filter should be applied or not.
func AlwaysFilterRDS(c context.Context) context.Context {
	return ds.AddRawFilters(c, func(c context.Context, rds ds.RawInterface) ds.RawInterface {
		shardFns, _ := c.Value(&dsShardFunctionsKey).([]ShardFunction)

		sc := &supportContext{
			ds.GetKeyContext(c),
			c,
			mathrand.Get(c),
			shardFns,
		}

		v := c.Value(&dsTxnCacheKey)
		if v == nil {
			return &dsCache{rds, sc}
		}
		return &dsTxnCache{rds, v.(*dsTxnState), sc}
	})
}

// AddShardFunctions appends the provided shardFn functions to the internal list
// of shard functions. They are evaluated left to right, bottom to top.
//
// nil functions will cause a panic.
//
// So:
//   ctx = AddShardFunctions(ctx, A, B, C)
//   ctx = AddShardFunctions(ctx, D, E, F)
//
// Would evaulate `D, E, F, A, B, C`
func AddShardFunctions(c context.Context, shardFns ...ShardFunction) context.Context {
	cur, _ := c.Value(&dsShardFunctionsKey).([]ShardFunction)
	new := make([]ShardFunction, 0, len(cur)+len(shardFns))
	for _, fn := range shardFns {
		if fn == nil {
			panic("nil function provided to AddShardFunctions")
		}
	}
	return context.WithValue(c, &dsShardFunctionsKey, append(append(new, shardFns...), cur...))
}
