// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package proccache implements a simple in-memory cache that can be injected
// into a context and shared by all request handlers executing within a process.
// Same can be achieved by using global variables, but global state complicates
// unit testing.
package proccache

import (
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
	"golang.org/x/net/context"
)

// Getter is returned by Cached. See Cached for more details.
type Getter func(c context.Context) (interface{}, error)

// Maker is used by GetOrMake to make new cache item if previous one has expired.
// It returns a value to put in the cache along with its expiration duration
// (or 0 if it doesn't expire).
type Maker func() (interface{}, time.Duration, error)

// Warmer is used by Cached to make a new cache item if previous one has
// expired. It returns a value to put in the cache along with its expiration
// duration (or 0 if it doesn't expire). Unlike Maker, it doesn't expect the
// context and the key to be in the function closure and accepts them
// explicitly.
type Warmer func(c context.Context, key interface{}) (interface{}, time.Duration, error)

// Entry is returned by Get. It is a stored value along with its expiration
// time (that may have already passed). Zero expiration time means the item
// doesn't expire.
type Entry struct {
	Value interface{}
	Exp   time.Time
}

// Cache holds a mapping key -> (value, expiration time). The mapping is never
// cleared automatically, store only O(1) number of items there.
type Cache struct {
	lock  sync.Mutex
	cache map[interface{}]Entry
}

// Put adds a new item to the cache or replaces an existing one.
func (c *Cache) Put(key, value interface{}, exp time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.cache == nil {
		c.cache = make(map[interface{}]Entry, 1)
	}
	c.cache[key] = Entry{value, exp}
}

// Get returns a stored item or nil if no such item.
func (c *Cache) Get(key interface{}) *Entry {
	c.lock.Lock()
	defer c.lock.Unlock()
	if e, ok := c.cache[key]; ok {
		return &e
	}
	return nil
}

type contextKey int

// Use injects instance of Cache in the context.
func Use(c context.Context, cache *Cache) context.Context {
	return context.WithValue(c, contextKey(0), cache)
}

// GetCache grabs instance of Cache stored in the context, or creates a new one
// if nothing is stored.
func GetCache(c context.Context) *Cache {
	if c, ok := c.Value(contextKey(0)).(*Cache); ok && c != nil {
		return c
	}
	return &Cache{}
}

// Put grabs an instance of Cache from the context and puts an item in it. Zero
// expiration duration means the item doesn't expire. If context doesn't have
// a cache installed, Put is noop.
func Put(c context.Context, key, value interface{}, exp time.Duration) {
	expTs := time.Time{}
	if exp != 0 {
		expTs = clock.Now(c).Add(exp)
	}
	GetCache(c).Put(key, value, expTs)
}

// Get returns an item from cache in the context if it hasn't expired yet. If
// there's no such item or it has expired, returns (nil, false).
func Get(c context.Context, key interface{}) (value interface{}, ok bool) {
	e := GetCache(c).Get(key)
	if e != nil && (e.Exp.IsZero() || clock.Now(c).Before(e.Exp)) {
		return e.Value, true
	}
	return nil, false
}

// GetOrMake attempts to grab an item from the cache. If it's not there, it
// calls a supplied callback to generate it.
func GetOrMake(c context.Context, key interface{}, maker Maker) (interface{}, error) {
	if value, ok := Get(c, key); ok {
		return value, nil
	}
	value, exp, err := maker()
	if err != nil {
		return nil, err
	}
	Put(c, key, value, exp)
	return value, nil
}

// Cached returns a getter function that extracts an item from the cache (if it
// is there) or calls a supplied callback to initialize and put it there.
// Intended to be used like a decorator for top level functions. The getter
// returns a error only if cache initialization callback (Warmer) returns
// a error. Warmer callback is not protected by any locks, and it may be called
// concurrently (and the cache will have the result of invocation that finished
// last, whatever it may be). Implement synchronization inside the callback
// itself if needed.
func Cached(key interface{}, warmer Warmer) Getter {
	return func(c context.Context) (interface{}, error) {
		return GetOrMake(c, key, func() (interface{}, time.Duration, error) {
			return warmer(c, key)
		})
	}
}
