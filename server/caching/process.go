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
	"sync/atomic"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/caching/lru"
)

type registeredCache struct {
	// TODO(vadimsh): Add a name here and start exporting LRU cache sizes as
	// monitoring metrics.
	capacity int
}

var (
	processCacheKey       = "server.caching Process Cache"
	registeredCaches      []registeredCache
	registrationForbidden uint32
)

// Handle is indirect pointer to a registered process cache.
//
// Grab it via RegisterProcessCache during module init time, and use its
// LRU() method to access an actual LRU cache associated with this handle.
//
// The cache itself lives inside the context. See WithProcessCacheData.
type Handle int

// LRU returns global lru.Cache referenced by this handle.
//
// If the context doesn't have ProcessCacheData installed, this will panic.
func (h Handle) LRU(c context.Context) *lru.Cache {
	return c.Value(&processCacheKey).(*ProcessCacheData).caches[int(h)]
}

// RegisterProcessCache is used during init time to declare an intent that a
// package wants to use a process-global LRU cache of given capacity (or 0 for
// unlimited).
//
// The actual cache itself will be stored in ProcessCacheData inside the
// context.
func RegisterProcessCache(capacity int) Handle {
	if atomic.LoadUint32(&registrationForbidden) == 1 {
		// Note: this panic may happen if NewProcessCacheData is called during
		// init time, before some RegisterProcessCache call. Use NewProcessCacheData
		// only from main() (or code under main), not in init().
		panic("can't call RegisterProcessCache after NewProcessCacheData is called")
	}
	registeredCaches = append(registeredCaches, registeredCache{capacity})
	return Handle(len(registeredCaches) - 1)
}

// ProcessCacheData holds all process-cached data (internally).
//
// It is opaque to the API users. Use NewProcessCacheData in your main() or
// below (i.e. any other place _other_ than init()) to allocate it, then inject
// it into the context via WithProcessCacheData, and finally access it through
// handles registered during init() time via RegisterProcessCache to get
// a reference to an actual lru.Cache.
//
// Each instance of ProcessCacheData is its own universe of global data. This is
// useful in unit tests as replacement for global variables.
type ProcessCacheData struct {
	caches []*lru.Cache // handle => lru.Cache, never nil once initialized
}

// NewProcessCacheData allocates and initializes all registered LRU caches.
//
// It returns a fat stateful object that holds all the cached data. Retain it
// and share between requests etc. to actually benefit from the cache.
//
// NewProcessCacheData must be called after init() time (either in main or
// code called from main).
func NewProcessCacheData() *ProcessCacheData {
	// Once NewProcessCacheData is used (after init-time is done), we forbid
	// registering new caches. All RegisterProcessCache calls should happen during
	// module init time.
	atomic.StoreUint32(&registrationForbidden, 1)
	d := &ProcessCacheData{
		caches: make([]*lru.Cache, len(registeredCaches)),
	}
	for i, params := range registeredCaches {
		d.caches[i] = lru.New(params.capacity)
	}
	return d
}

// WithProcessCacheData installs a process-global cache storage into the
// supplied context.
//
// The actual storage space ('data' argument) for the cache must be allocated
// via NewProcessCacheData and retained in some global variable to benefit from
// caching.
func WithProcessCacheData(c context.Context, data *ProcessCacheData) context.Context {
	if data.caches == nil {
		panic("use NewProcessCacheData to allocated ProcessCacheData")
	}
	return context.WithValue(c, &processCacheKey, data)
}

// WithEmptyProcessCache installs an empty process-global cache storage into
// the context.
//
// Useful in main() when initializing a root context (used as a basis for all
// other contexts) or in unit tests to "reset" the cache state.
//
// Note that using WithEmptyProcessCache when initializing per-request context
// makes no sense, since each request will get its own cache.
func WithEmptyProcessCache(c context.Context) context.Context {
	return WithProcessCacheData(c, NewProcessCacheData())
}
