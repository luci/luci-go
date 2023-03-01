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
	"context"
	"errors"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/common/data/caching/lru"
)

var (
	// ErrNoProcessCache is returned by Fetch if the context doesn't have
	// ProcessCacheData.
	//
	// This usually happens in tests. Use WithEmptyProcessCache to prepare the
	// context.
	ErrNoProcessCache = errors.New("no process cache is installed in the context, use WithEmptyProcessCache")
)

type registeredCache struct {
	// Produces an empty *lru.Cache[...]. Has to return `any` since factories for
	// different types of caches are all registered in a single registry.
	factory func() any

	// TODO(vadimsh): Add a name here and start exporting LRU cache sizes as
	// monitoring metrics.
}

var (
	processCacheKey       = "server.caching Process Cache"
	registeredCaches      []registeredCache
	registeredSlots       uint32
	registrationForbidden uint32
)

func finishInitTime() {
	atomic.StoreUint32(&registrationForbidden, 1)
}

func checkStillInitTime() {
	if atomic.LoadUint32(&registrationForbidden) == 1 {
		// Note: this panic may happen if NewProcessCacheData is called during
		// init time, before some RegisterLRUCache call. Use NewProcessCacheData
		// only from main() (or code under main), not in init().
		panic("can't call RegisterLRUCache/RegisterCacheSlot after NewProcessCacheData is called")
	}
}

// LRUHandle is indirect pointer to a registered LRU process cache.
//
// Grab it via RegisterLRUCache during module init time, and use its LRU()
// method to access an actual LRU cache associated with this handle.
//
// The cache itself lives inside a context. See WithProcessCacheData.
type LRUHandle[K comparable, V any] struct{ h int }

// Valid returns true if h was initialized.
func (h LRUHandle[K, V]) Valid() bool { return h.h != 0 }

// LRU returns global lru.Cache referenced by this handle.
//
// Returns nil if the context doesn't have ProcessCacheData.
func (h LRUHandle[K, V]) LRU(ctx context.Context) *lru.Cache[K, V] {
	if h.h == 0 {
		panic("calling LRU on a uninitialized LRUHandle")
	}
	pcd, _ := ctx.Value(&processCacheKey).(*ProcessCacheData)
	if pcd == nil {
		return nil
	}
	return pcd.caches[h.h-1].(*lru.Cache[K, V])
}

// RegisterLRUCache is used during init time to declare an intent that a package
// wants to use a process-global LRU cache of given capacity (or 0 for
// unlimited).
//
// The actual cache itself will be stored in ProcessCacheData inside a context.
func RegisterLRUCache[K comparable, V any](capacity int) LRUHandle[K, V] {
	checkStillInitTime()
	registeredCaches = append(registeredCaches, registeredCache{
		factory: func() any { return lru.New[K, V](capacity) },
	})
	return LRUHandle[K, V]{len(registeredCaches)}
}

// SlotHandle is indirect pointer to a registered process cache slot.
//
// Such slot holds one arbitrary value, alongside its expiration time. Useful
// for representing global singletons that occasionally need to be refreshed.
//
// Grab it via RegisterCacheSlot during module init time, and use its Fetch()
// method to access the value, potentially refreshing it, if necessary.
//
// The value itself lives inside a context. See WithProcessCacheData.
type SlotHandle struct{ h uint32 }

// Valid returns true if h was initialized.
func (h SlotHandle) Valid() bool { return h.h != 0 }

// FetchCallback knows how to grab a new value for the cache slot (if prev is
// nil) or refresh the known one (if prev is not nil).
//
// If the returned expiration time is 0, the value is considered non-expirable.
// If the returned expiration time is <0, the value will be refetched on the
// next access. This is sometimes useful in tests that "freeze" time.
type FetchCallback func(prev any) (updated any, exp time.Duration, err error)

// Fetch returns the cached data, if it is available and fresh, or attempts to
// refresh it by calling the given callback.
//
// Returns ErrNoProcessCache if the context doesn't have ProcessCacheData.
func (h SlotHandle) Fetch(ctx context.Context, cb FetchCallback) (any, error) {
	if h.h == 0 {
		panic("calling Fetch on a uninitialized SlotHandle")
	}
	pcd, _ := ctx.Value(&processCacheKey).(*ProcessCacheData)
	if pcd == nil {
		return nil, ErrNoProcessCache
	}
	return pcd.slots[h.h-1].Get(ctx, lazyslot.Fetcher(cb))
}

// RegisterCacheSlot is used during init time to preallocate a place for the
// cache global variable.
//
// The actual cache itself will be stored in ProcessCacheData inside a context.
func RegisterCacheSlot() SlotHandle {
	checkStillInitTime()
	return SlotHandle{atomic.AddUint32(&registeredSlots, 1)}
}

// ProcessCacheData holds all process-cached data (internally).
//
// It is opaque to the API users. Use NewProcessCacheData in your main() or
// below (i.e. any other place _other_ than init()) to allocate it, then inject
// it into the context via WithProcessCacheData, and finally access it through
// handles registered during init() time via RegisterLRUCache to get a reference
// to an actual lru.Cache.
//
// Each instance of ProcessCacheData is its own universe of global data. This is
// useful in unit tests as replacement for global variables.
type ProcessCacheData struct {
	caches []any           // handle => *lru.Cache, never nil once initialized
	slots  []lazyslot.Slot // handle => corresponding slot
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
	// registering new caches. All RegisterLRUCache/RegisterCacheSlot calls should
	// happen during module init time.
	finishInitTime()
	d := &ProcessCacheData{
		caches: make([]any, len(registeredCaches)),
		slots:  make([]lazyslot.Slot, registeredSlots),
	}
	for i, params := range registeredCaches {
		d.caches[i] = params.factory()
	}
	return d
}

// WithEmptyProcessCache installs an empty process-global cache storage into
// the context.
//
// Useful in main() when initializing a root context (used as a basis for all
// other contexts) or in unit tests to "reset" the cache state.
//
// Note that using WithEmptyProcessCache when initializing per-request context
// makes no sense, since each request will get its own cache. Instead allocate
// the storage cache area via NewProcessCacheData(), retain it in some global
// variable and install into per-request context via WithProcessCacheData.
func WithEmptyProcessCache(ctx context.Context) context.Context {
	return WithProcessCacheData(ctx, NewProcessCacheData())
}

// WithProcessCacheData installs an existing process-global cache storage into
// the supplied context.
//
// It must be allocated via NewProcessCacheData().
func WithProcessCacheData(ctx context.Context, data *ProcessCacheData) context.Context {
	if data.caches == nil {
		panic("use NewProcessCacheData to allocate ProcessCacheData")
	}
	return context.WithValue(ctx, &processCacheKey, data)
}
