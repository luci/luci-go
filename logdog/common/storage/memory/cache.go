// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"sync"
	"time"

	"github.com/luci/luci-go/logdog/common/storage/caching"

	"golang.org/x/net/context"
)

// Cache is an in-memory caching.Cache implementation.
type Cache struct {
	mu       sync.Mutex
	cacheMap map[cacheKey]caching.Item
}

var _ caching.Cache = (*Cache)(nil)

type cacheKey struct {
	schema string
	typ    string
	key    string
}

// Put implements caching.Cache.
func (c *Cache) Put(ctx context.Context, exp time.Duration, items ...*caching.Item) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cacheMap == nil {
		c.cacheMap = make(map[cacheKey]caching.Item)
	}

	for _, itm := range items {
		c.cacheMap[c.keyForItem(itm)] = *itm
	}
}

// Get implements caching.Cache.
func (c *Cache) Get(ctx context.Context, items ...*caching.Item) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, itm := range items {
		if cacheItem, ok := c.cacheMap[c.keyForItem(itm)]; ok {
			itm.Data = append([]byte{}, cacheItem.Data...)
		} else {
			itm.Data = nil
		}
	}
}

// Delete deletes a set of keys from the cache.
func (c *Cache) Delete(schema, typ string, key ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, k := range key {
		delete(c.cacheMap, cacheKey{schema, typ, k})
	}
}

func (*Cache) keyForItem(itm *caching.Item) cacheKey { return cacheKey{itm.Schema, itm.Type, itm.Key} }
