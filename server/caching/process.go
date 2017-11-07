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
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/caching/lru"
)

type registeredCache struct {
	key      string
	capacity int
}

var registeredCaches []registeredCache

// Handle is indirect pointer to a registered process cache.
//
// Grab it via RegisterProcessCache during module init time, and pass it to
// ProcessCache to get an lru.Cache.
type Handle int

// RegisterProcessCache is used during init time to declare an intent that a
// package wants to use a process-global LRU cache of given capacity (or 0 for
// unlimited).
//
// The actual cache itself will be stored in ProcessCacheData inside the
// context.
func RegisterProcessCache(key string, capacity int) Handle {
	// TODO: check the key is not used yet.
	registeredCaches = append(registeredCaches, registeredCache{key, capacity})
	return Handle(len(registeredCaches) - 1)
}

var processCacheKey = "server.caching Process Cache"

// ProcessCacheData internally holds all process-cached data.
//
// It is opaque to the API users. Use package-level function ProcessCache to
// get a reference to an actual lru.Cache.
//
// Each instance of ProcessCacheData is its own universe of global data. This is
// useful in unit tests as replacement for global variables.
type ProcessCacheData struct {
	caches []*lru.Cache // handle => lru.Cache
}

// WithProcessCache installs a process-global cache storage into the supplied
// Context.
//
// It can be used as a fast layer of caching for information whose value extends
// past the lifespan of a single request.
//
// For information whose cached value is limited to a single request, use
// RequestCache.
func WithProcessCache(c context.Context, data *ProcessCacheData) context.Context {
	data.caches = make([]*lru.Cache, len(registeredCaches))
	for i, params := range registeredCaches {
		data.caches[i] = lru.New(params.capacity)
	}
	return context.WithValue(c, &processCacheKey, data)
}

// ProcessCache returns process-global cache that is installed into the Context.
//
// If no process-global cache is installed, this will panic.
func ProcessCache(c context.Context, h Handle) *lru.Cache {
	pc, _ := c.Value(&processCacheKey).(*ProcessCacheData)
	if pc == nil {
		panic("server/caching: no process cache installed in Context")
	}
	return pc.caches[int(h)]
}
