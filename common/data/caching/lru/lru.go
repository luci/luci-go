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
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/sync/mutexpool"
)

// Item is a Cache item. It's used when interfacing with Mutate.
type Item struct {
	// Value is the item's value. It may be nil.
	Value any
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

// Maker generates a new value. It is used by GetOrCreate, and is run while a
// lock on that value's key is held, but not while the larger Cache is locked,
// so it is safe for this to take some time.
//
// The returned value, v, will be stored in the cache after Maker exits.
//
// If the Maker returns an error, the returned value will not be cached, and
// the error will be returned by GetOrCreate.
type Maker func() (v any, exp time.Duration, err error)

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
	// maxSize, if >0, is the maximum number of elements that can reside in the
	// LRU. If 0, elements will not be evicted based on count. This creates a flat
	// cache that may never shrink.
	maxSize int

	// lock is a lock protecting the Cache's members.
	lock sync.RWMutex

	// cache is a map of elements. It is used for lookup.
	//
	// Cache reads may be made while holding the read or write lock. Modifications
	// to cache require the write lock to be held.
	cache map[any]*cacheEntry

	// head is the first element in a linked list of least-recently-used elements.
	//
	// Each time an element is used, it is moved to the beginning of the list.
	// Consequently, items at the end of the list will be the least-recently-used
	// items.
	head *cacheEntry

	// tail is the last element in a linked list of least-recently-used elements.
	tail *cacheEntry

	// mp is a Mutex pool used in GetOrCreate to lock around individual keys.
	mp mutexpool.P
}

type cacheEntry struct {
	k, v       any
	expiry     time.Time
	next, prev *cacheEntry
}

// New creates a new LRU with given maximum size.
//
// If maxSize is <= 0, the LRU cache will have infinite capacity and will never
// prune LRU elements when adding new ones. Use Prune to reclaim memory occupied
// by expired elements.
func New(maxSize int) *Cache {
	return &Cache{
		maxSize: maxSize,
		cache:   make(map[any]*cacheEntry),
	}
}

// Peek fetches the element associated with the supplied key without updating
// the element's recently-used standing.
//
// Peek uses the cache read lock.
func (c *Cache) Peek(ctx context.Context, key any) (any, bool) {
	now := clock.Now(ctx)

	c.lock.RLock()
	defer c.lock.RUnlock()

	if ent := c.cache[key]; ent != nil {
		if !c.hasExpired(now, ent) {
			return ent.v, true
		}
	}
	return nil, false
}

// Get fetches the element associated with the supplied key, updating its
// recently-used standing.
//
// Get uses the cache read/write lock.
func (c *Cache) Get(ctx context.Context, key any) (any, bool) {
	now := clock.Now(ctx)

	// We need a Read/Write lock here because if the entry is present, we are
	// going to need to promote it "lru".
	c.lock.Lock()
	defer c.lock.Unlock()

	if ent := c.cache[key]; ent != nil {
		if !c.hasExpired(now, ent) {
			c.moveToFrontLocked(ent)
			return ent.v, true
		}
		c.deleteLocked(ent)
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
func (c *Cache) Put(ctx context.Context, key, value any, exp time.Duration) (prev any, had bool) {
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
func (c *Cache) Mutate(ctx context.Context, key any, gen Generator) (value any, ok bool) {
	now := clock.Now(ctx)

	c.lock.Lock()
	defer c.lock.Unlock()

	ent := c.cache[key]

	// Populate `it` only if there's a non-expired entry. Otherwise pretend
	// there's no existing item.
	var it *Item
	if ent != nil && !c.hasExpired(now, ent) {
		it = &Item{Value: ent.v}
		if !ent.expiry.IsZero() {
			// Get remaining lifetime of this entry.
			it.Exp = ent.expiry.Sub(now)
		}
	}

	// Invoke our generator.
	it = gen(it)

	if it == nil {
		// The generator indicated that no value should be stored, and any
		// current value should be purged.
		if ent != nil {
			c.deleteLocked(ent)
		}
		return nil, false
	}

	// Generate our entry.
	if ent == nil {
		// This is a new entry. Put into the map and place at the front in the
		// linked list.
		ent = &cacheEntry{k: key, v: it.Value}
		c.cache[key] = ent
		c.pushToFrontLocked(ent)
		// Because we added a new entry, we need to perform a pruning round.
		if c.maxSize > 0 {
			c.pruneSizeLocked(c.maxSize)
		}
	} else {
		// The element already exists. Updates its value and bump it to the front.
		ent.v = it.Value
		c.moveToFrontLocked(ent)
	}

	// Bump the expiration time.
	if it.Exp <= 0 {
		ent.expiry = time.Time{}
	} else {
		ent.expiry = now.Add(it.Exp)
	}

	return it.Value, true
}

// Remove removes an entry from the cache. If the key is present, its current
// value will be returned; otherwise, nil will be returned.
//
// Remove uses the cache read/write lock.
func (c *Cache) Remove(key any) (val any, has bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if ent, ok := c.cache[key]; ok {
		val, has = ent.v, true
		c.deleteLocked(ent)
	}
	return
}

// GetOrCreate retrieves the current value of key. If no value exists for key,
// GetOrCreate will lock around key and invoke the supplied Maker to generate
// a new value.
//
// If multiple concurrent operations invoke GetOrCreate at the same time, they
// will serialize, and at most one Maker will be invoked at a time. If the Maker
// succeeds, it is more likely that the first operation will generate and
// install a value, and the other operations will all quickly retrieve that
// value once unblocked.
//
// If the Maker returns an error, the error will be returned by GetOrCreate and
// no modifications will be made to the Cache. If Maker returns a nil error, the
// value that it returns will be added into the Cache and returned to the
// caller.
//
// Note that the Cache's lock will not be held while the Maker is running.
// Operations on to the Cache using other methods will not lock around
// key. This will not interfere with GetOrCreate.
func (c *Cache) GetOrCreate(ctx context.Context, key any, fn Maker) (v any, err error) {
	// First, check if the value is the cache. We don't need to hold the item's
	// Mutex for this.
	var ok bool
	if v, ok = c.Get(ctx, key); ok {
		return v, nil
	}

	// The value is currently not cached, so we will generate it.
	c.mp.WithMutex(key, func() {
		// Has the value been cached since we obtained the key's lock?
		if v, ok = c.Get(ctx, key); ok {
			return
		}

		// Generate a new value.
		var exp time.Duration
		if v, exp, err = fn(); err != nil {
			// The Maker returned an error, so do not add the value to the cache.
			return
		}

		// Add the generated value to the cache.
		c.Put(ctx, key, v, exp)
	})
	return
}

// Create write-locks around key and invokes the supplied Maker to generate a
// new value.
//
// If multiple concurrent operations invoke GetOrCreate or Create at the same
// time, they will serialize, and at most one Maker will be invoked at a time.
// If the Maker succeeds, it is more likely that the first operation will
// generate and install a value, and the other operations will all quickly
// retrieve that value once unblocked.
//
// If the Maker returns an error, the error will be returned by Create and
// no modifications will be made to the Cache. If Maker returns a nil error, the
// value that it returns will be added into the Cache and returned to the
// caller.
//
// Note that the Cache's lock will not be held while the Maker is running.
// Operations on to the Cache using other methods will not lock around
// key. This will not interfere with Create.
func (c *Cache) Create(ctx context.Context, key any, fn Maker) (v any, err error) {
	c.mp.WithMutex(key, func() {
		// Generate a new value.
		var exp time.Duration
		if v, exp, err = fn(); err != nil {
			// The Maker returned an error, so do not add the value to the cache.
			return
		}

		// Add the generated value to the cache.
		c.Put(ctx, key, v, exp)
	})
	return
}

// Prune iterates through entries in the Cache and prunes any which are expired.
func (c *Cache) Prune(ctx context.Context) {
	now := clock.Now(ctx)

	c.lock.Lock()
	defer c.lock.Unlock()

	c.pruneExpiredLocked(now)
}

// Reset clears the full contents of the cache.
//
// Purge uses the cache read/write lock.
func (c *Cache) Reset() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.cache = make(map[any]*cacheEntry)
	c.head = nil
	c.tail = nil
}

// Len returns the number of entries in the cache.
//
// Len uses the cache read lock.
func (c *Cache) Len() (size int) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return len(c.cache)
}

// pruneSizeLocked prunes LRU elements until it has `size` items or less.
//
// Its write lock must be held by the caller.
func (c *Cache) pruneSizeLocked(size int) {
	for e := c.tail; e != nil && len(c.cache) > size; e = c.tail {
		c.deleteLocked(e)
	}
}

// pruneExpiredLocked prunes any entries that have expired.
func (c *Cache) pruneExpiredLocked(now time.Time) {
	for e := c.head; e != nil; {
		next := e.next
		if c.hasExpired(now, e) {
			c.deleteLocked(e)
		}
		e = next
	}
}

// hasExpired returns whether this entry has expired given the current time.
//
// An entry is expired if it has an expiration time, and the current time is >=
// the expiration time.
//
// Will return false if the entry has no expiration, or if the entry is not
// expired.
func (c *Cache) hasExpired(now time.Time, ent *cacheEntry) bool {
	return !(ent.expiry.IsZero() || now.Before(ent.expiry))
}

// pushToFrontLocked adds a new entry to the front of the linked list.
func (c *Cache) pushToFrontLocked(e *cacheEntry) {
	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

// moveToFrontLocked moves an entry to the front of the linked list.
func (c *Cache) moveToFrontLocked(e *cacheEntry) {
	if c.head == e {
		return
	}
	if c.tail == e {
		c.tail = e.prev
	}

	if e.next != nil {
		e.next.prev = e.prev
	}
	if e.prev != nil {
		e.prev.next = e.next
	}

	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
}

// deleteLocked removes the entry from the cache.
func (c *Cache) deleteLocked(e *cacheEntry) {
	delete(c.cache, e.k)

	if c.head == e {
		c.head = e.next
	}
	if c.tail == e {
		c.tail = e.prev
	}

	if e.next != nil {
		e.next.prev = e.prev
	}
	if e.prev != nil {
		e.prev.next = e.next
	}

	// Help GC.
	e.prev = nil
	e.next = nil
}
