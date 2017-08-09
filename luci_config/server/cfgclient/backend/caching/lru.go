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

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/sync/mutexpool"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/identity"

	"go.chromium.org/luci/luci_config/server/cfgclient/backend"

	"golang.org/x/net/context"
)

// LRUBackend wraps b, applying the LRU's caching to its results.
//
// If the LRU isn't fully configured, Backend will return b directly and not
// apply any caching.
func LRUBackend(b backend.B, size int, exp time.Duration) backend.B {
	// If our parameters are disabled, just return the underlying Backend.
	if size <= 0 || exp <= 0 {
		return b
	}

	lb := LRU{
		Cache:      lru.New(size),
		Expiration: exp,
	}
	return lb.Wrap(b)
}

// LRU manages configuration caching in an LRU cache.
//
// All configurations are stored side-by-side in the same LRU cache. Results
// bound to user accounts are cached alongside their current user IDs for
// consistent and safe storage and retrieval.
//
// Concurrent accesses to the same configuration will block pending a single
// resolution, reducing configuration burst traffic.
type LRU struct {
	// Cache is the LRU cache to store configurations in.
	Cache *lru.Cache
	// Expiration is the maximum age of a configuration before it expires from
	// the LRU cache.
	Expiration time.Duration

	mp mutexpool.P
}

// Wrap wraps b, applying the LRU's caching to its results.
func (lb *LRU) Wrap(b backend.B) backend.B {
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
			lb.mp.WithMutex(cacheKey, func() {
				// First, see if the value is already in the cache.
				v, _ := lb.Cache.Get(c, cacheKey)
				if v != nil {
					ret = v.(*Value)
					return
				}

				// The value wasn't in the cache. Resolve and cache it.
				if ret, err = l(c, key, nil); err != nil {
					return
				}

				lb.Cache.Put(c, cacheKey, ret, lb.Expiration)
				return
			})
			return
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
		key.ConfigSet,
		key.Path,
		string(key.GetAllTarget),
	}, ":"))
}
