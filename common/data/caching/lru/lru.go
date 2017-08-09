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

// Package lru provides least-recently-used (LRU) cache.
package lru

import (
	"container/list"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"

	"golang.org/x/net/context"
)

// snapshot is a snapshot of the contents of the Cache.
type snapshot map[interface{}]interface{}

type cacheEntry struct {
	k, v interface{}

	// expiry is the time when this entry expires. It will be 0 if the entry
	// never expires.
	expiry time.Time
}

// Item is a Cache item. It's used when interfacing with Mutate.
type Item struct {
	// Value is the item's value. It may be nil.
	Value interface{}

	// Exp is the item's expiration.
	Exp time.Duration
}

// Generator generates a new value, given the current value and its remaining
// expiration time.
//
// The returned value will be stored in the cache after the Generator exits,
// even if it is nil.
//
// The Generator is executed while the Cache's lock is held.
//
// If the returned Item is nil, no item will be retained, and the current
// value (if any) will be purged. If the returned Item's expiration is <=0,
// the returned Item will never expire. Otherwise, the Item will expire in the
// future.
type Generator func(it *Item) *Item

// Cache is a least-recently-used (LRU) cache implementation. The cache stores
// key-value mapping entries up to a size limit. If more items are added past
// that limit, the entries that have have been referenced least recently will be
// evicted.
//
// A Cache must be constructed using the New method.
//
// Cache is safe for concurrent access, using a read-write mutex to allow
// multiple non-mutating readers (Peek) or only one mutating reader/writer (Get,
// Put, Mutate).
type Cache struct {
	// size, if >0, is the maximum number of elements that can reside in the LRU.
	// If 0, elements will not be evicted based on count. This creates a flat
	// cache that may never shrink.
	size int

	// lock is a lock protecting the Cache's members.
	lock sync.RWMutex

	// cache is a map of elements. It is used for lookup.
	//
	// Cache reads may be made while holding the read or write lock. Modifications
	// to cache require the write lock to be held.
	cache map[interface{}]*list.Element
	// lru is a List of least-recently-used elements. Each time an element is
	// used, it is moved to the beginning of the List. Consequently, items at the
	// end of the list will be the least-recently-used items.
	//
	// Accesses to lru require the write lock to be held.
	lru list.List
}

// New creates a new, locking LRU cache with the specified size.
//
// If size is <= 0, the LRU cache will have infinite capacity, and will never
// prune elements.
func New(size int) *Cache {
	c := Cache{
		size: size,
	}
	c.Reset()
	return &c
}

// Peek fetches the element associated with the supplied key without updating
// the element's recently-used standing.
//
// Peek uses the cache read lock.
func (c *Cache) Peek(ctx context.Context, key interface{}) (interface{}, bool) {
	now := clock.Now(ctx)

	c.lock.RLock()
	defer c.lock.RUnlock()

	if e := c.cache[key]; e != nil {
		ent := e.Value.(*cacheEntry)
		if !c.isEntryExpired(now, ent) {
			return ent.v, true
		}
	}
	return nil, false
}

// Get fetches the element associated with the supplied key, updating its
// recently-used standing.
//
// Get uses the cache read/write lock.
func (c *Cache) Get(ctx context.Context, key interface{}) (interface{}, bool) {
	now := clock.Now(ctx)

	// We need a Read/Write lock here because if the entry is present, we are
	// going to need to promote it "lru".
	c.lock.Lock()
	defer c.lock.Unlock()

	if e := c.cache[key]; e != nil {
		ent := e.Value.(*cacheEntry)
		if !c.isEntryExpired(now, ent) {
			c.lru.MoveToFront(e)
			return ent.v, true
		}
		c.evictEntryLocked(e)
	}

	return nil, false
}

// Put adds a new value to the cache. The value in the cache will be replaced
// regardless of whether an item with the same key already existed.
//
// If the supplied expiration is <=0, the item will not have a time-based
// expiration associated with it, although it still may be pruned if it is
// not used.
//
// Put uses the cache read/write lock.
//
// Returns whether not a value already existed for the key.
//
// The new item will be considered most recently used.
func (c *Cache) Put(ctx context.Context, key, value interface{}, exp time.Duration) (prev interface{}, had bool) {
	c.Mutate(ctx, key, func(cur *Item) *Item {
		if cur != nil {
			prev, had = cur.Value, true
		}
		return &Item{value, exp}
	})
	return
}

// Mutate adds a value to the cache, using a Generator to create the value.
//
// Mutate uses the cache read/write lock.
//
// The Generator will receive the current value, or nil if there is no current
// value. It returns the new value, or nil to remove this key from the cache.
//
// The Generator is called while the cache's lock is held. This means that
// the Generator MUST NOT call any cache methods during its execution, as
// doing so will result in deadlock/panic.
//
// Returns the value that was put in the cache, which is the value returned
// by the Generator. "ok" will be true if the value is in the cache and valid.
//
// The key will be considered most recently used regardless of whether it was
// put.
func (c *Cache) Mutate(ctx context.Context, key interface{}, gen Generator) (interface{}, bool) {
	now := clock.Now(ctx)

	c.lock.Lock()
	defer c.lock.Unlock()

	// Get the current entry.
	var it *Item
	var ent *cacheEntry

	e := c.cache[key]
	if e != nil {
		// If "e" is expired, purge it and pretend that it doesn't exist.
		ent = e.Value.(*cacheEntry)

		if !c.isEntryExpired(now, ent) {
			it = &Item{
				Value: ent.v,
			}
			if !ent.expiry.IsZero() {
				// Get remaining lifetime of this entry.
				it.Exp = ent.expiry.Sub(now)
			}
		}
	}

	// Invoke our generator.
	it = gen(it)
	if it == nil {
		// The generator indicted that no value should be stored, and any
		// current value should be purged.
		if e != nil {
			c.evictEntryLocked(e)
		}
		return nil, false
	}

	// Generate our entry and calculate our expiration time.
	if ent == nil {
		ent = &cacheEntry{key, it.Value, time.Time{}}
	} else {
		ent.v = it.Value
	}

	// Set/update expiry.
	if it.Exp <= 0 {
		ent.expiry = time.Time{}
	} else {
		ent.expiry = now.Add(it.Exp)
	}

	if e == nil {
		// The key doesn't currently exist. Create a new one and place it at the
		// front.
		e = c.lru.PushFront(ent)
		c.cache[key] = e

		// Because we added a new entry, we need to perform a pruning round.
		c.pruneSizeLocked()
	} else {
		// The element already exists. Visit it.
		c.lru.MoveToFront(e)
	}
	return it.Value, true
}

// Remove removes an entry from the cache. If the key is present, its current
// value will be returned; otherwise, nil will be returned.
//
// Remove uses the cache read/write lock.
func (c *Cache) Remove(key interface{}) (val interface{}, has bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if e, ok := c.cache[key]; ok {
		val, has = e.Value.(*cacheEntry).v, true
		c.evictEntryLocked(e)
	}
	return
}

// Prune iterates through entries in the Cache and prunes any which are
// expired.
func (c *Cache) Prune(ctx context.Context) {
	now := clock.Now(ctx)

	c.lock.Lock()
	defer c.lock.Unlock()

	c.pruneExpiryLocked(now)
}

// Reset clears the full contents of the cache.
//
// Purge uses the cache read/write lock.
func (c *Cache) Reset() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.cache = make(map[interface{}]*list.Element)
	c.lru.Init()
}

// Len returns the number of entries in the cache.
//
// Len uses the cache read lock.
func (c *Cache) Len() (size int) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return len(c.cache)
}

// snapshot returns a snapshot map of the cache's entries.
func (c *Cache) snapshot() (ss snapshot) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if len(c.cache) > 0 {
		ss = make(snapshot)
		for k, e := range c.cache {
			ss[k] = e.Value.(*cacheEntry).v
		}
	}
	return
}

// pruneSizeLocked prunes LRU elements until its heuristic is satisfied. Its
// write lock must be held by the caller.
func (c *Cache) pruneSizeLocked() {
	// There is no size constraint.
	if c.size <= 0 {
		return
	}

	// Prune the oldest entries until we're within our size constraint.
	for e := c.lru.Back(); e != nil; e = c.lru.Back() {
		if len(c.cache) <= c.size {
			return
		}
		c.evictEntryLocked(e)
	}
}

func (c *Cache) pruneExpiryLocked(now time.Time) {
	// Iterate front-to-back and prune any entries that have expired.
	var cur *list.Element
	e := c.lru.Front()
	for e != nil {
		// Advance to the next element before pruning the current. Otherwise,
		// the Next() pointer may be invalid.
		cur, e = e, e.Next()

		if c.isEntryExpired(now, cur.Value.(*cacheEntry)) {
			c.evictEntryLocked(cur)
		}
	}
}

// isEntryExpired returns whether this entry is expired given the current time.
//
// An entry is expired if it has an expiration time, and the current time is >=
// the expiration time.
//
// Will return false if the entry has no expiration, or if the entry is not
// expired.
func (c *Cache) isEntryExpired(now time.Time, ent *cacheEntry) bool {
	return !(ent.expiry.IsZero() || now.Before(ent.expiry))
}

func (c *Cache) evictEntryLocked(e *list.Element) {
	delete(c.cache, e.Value.(*cacheEntry).k)
	c.lru.Remove(e)
}
