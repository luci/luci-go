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

package caching

import (
	"strings"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config/server/cfgclient/backend"

	"golang.org/x/net/context"
)

// LRUBackend wraps b, applying the additional cache to its results.
//
// Do NOT chain multiple LRUBackend in the same backend chain. An LRUBackend
// holds a LRU-wide lock on its cache key when it looks up its value, and if
// a second entity attempts to lock that same key during its lookup chain, the
// lookup will deadlock.
//
// If the supplied expiration is <= 0, no additional caching will be performed.
func LRUBackend(b backend.B, cache *lru.Cache, exp time.Duration) backend.B {
	// If our parameters are disabled, just return the underlying Backend.
	if exp <= 0 {
		return b
	}

	// mp is used to block concurrent accesses to the same cache key.
	return &Backend{
		B: b,
		CacheGet: func(c context.Context, key Key, l Loader) (ret *Value, err error) {
			// Generate the identity component of our cache key.
			//
			// For AsAnonymous and AsService access, we cache based on a singleton
			// identity. For AsUser, however, we cache on a per-user basis.
			var id identity.Identity
			if key.Authority == backend.AsUser {
				id = auth.CurrentIdentity(c)
			}

			cacheKey := mkLRUCacheKey(&key, id)

			// Lock around this key. This will ensure that concurrent accesses to the
			// same key will not result in multiple redundant fetches.
			v, err := cache.GetOrCreate(c, cacheKey, func() (interface{}, time.Duration, error) {
				// The value wasn't in the cache. Resolve and cache it.
				ret, err := l(c, key, nil)
				if err != nil {
					return nil, 0, err
				}
				return ret, exp, nil
			})
			if err != nil {
				return nil, err
			}
			return v.(*Value), nil
		},
	}
}

type lruCacheKey string

func mkLRUCacheKey(key *Key, id identity.Identity) lruCacheKey {
	return lruCacheKey(strings.Join([]string{
		key.Authority.String(),
		string(id),
		key.Schema,
		string(key.Op),
		string(key.ConfigSet),
		key.Path,
		string(key.GetAllTarget),
	}, ":"))
}
