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
	"context"

	ds "go.chromium.org/luci/gae/service/datastore"
)

var dsTxnCacheKey = "holds a *dsCache"
var dsShardFunctionsKey = "holds []ShardFunction"
var defaultImpl Cache = memcacheImpl{}

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
// It uses the given `impl` to actually do caching. If nil, uses GAE Memcache.
func FilterRDS(ctx context.Context, impl Cache) context.Context {
	if impl == nil {
		impl = defaultImpl
	}
	return ds.AddRawFilters(ctx, func(ctx context.Context, rds ds.RawInterface) ds.RawInterface {
		shardFns, _ := ctx.Value(&dsShardFunctionsKey).([]ShardFunction)

		sc := &supportContext{
			ds.GetKeyContext(ctx),
			ctx,
			impl,
			shardFns,
		}

		v := ctx.Value(&dsTxnCacheKey)
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
//
//	ctx = AddShardFunctions(ctx, A, B, C)
//	ctx = AddShardFunctions(ctx, D, E, F)
//
// Would evaluate `D, E, F, A, B, C`
func AddShardFunctions(ctx context.Context, shardFns ...ShardFunction) context.Context {
	cur, _ := ctx.Value(&dsShardFunctionsKey).([]ShardFunction)
	new := make([]ShardFunction, 0, len(cur)+len(shardFns))
	for _, fn := range shardFns {
		if fn == nil {
			panic("nil function provided to AddShardFunctions")
		}
	}
	return context.WithValue(ctx, &dsShardFunctionsKey, append(append(new, shardFns...), cur...))
}
