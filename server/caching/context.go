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

// Package caching implements common server object caches.
//
// Two caches are defined:
// - A process-global LRU cache, which may retain data in between requests.
// - A per-request cache, which can be installed into the Context that is
//   servicing an individual request, and will be purged at the end of that
//   request.
package caching

import (
	"go.chromium.org/luci/common/data/caching/lru"

	"golang.org/x/net/context"
)

var (
	processCacheKey = "server.caching Process Cache"
	requestCacheKey = "server.caching Per-Request Cache"
)

// WithProcessCache installs a process-global cache into the supplied Context.
//
// It can be used as a fast layer of caching for information whose value extends
// past the lifespan of a single request.
//
// For information whose cached value is limited to a single request, use
// RequestCache.
func WithProcessCache(c context.Context, cache *lru.Cache) context.Context {
	return context.WithValue(c, &processCacheKey, cache)
}

// ProcessCache returns process-global cache that is installed into the Context.
//
// If no process-global cache is installed, this will return panic.
func ProcessCache(c context.Context) *lru.Cache {
	pc, _ := c.Value(&processCacheKey).(*lru.Cache)
	if pc == nil {
		panic("server/caching: no process cache installed in Context")
	}
	return pc
}

// WithRequestCache initializes context-bound local cache and adds it to the
// Context.
//
// If the supplied cache is nil, a cache with unbounded size will be used. This
// is not necessarily bad, since the lifetime of the cache is still scoped to
// a single request.
//
// It can be used as a second fast layer of caching in front of memcache.
// It is never trimmed, only released at once upon the request completion.
func WithRequestCache(c context.Context, cache *lru.Cache) context.Context {
	if cache == nil {
		cache = lru.New(0)
	}
	return context.WithValue(c, &requestCacheKey, cache)
}

// RequestCache retrieves a per-request in-memory cache to the Context. If no
// request cache is installed, this will return nil.
//
// Note that while ProcessCache is required, RequestCache is optional.
func RequestCache(c context.Context) *lru.Cache {
	rc, _ := c.Value(&requestCacheKey).(*lru.Cache)
	return rc
}
