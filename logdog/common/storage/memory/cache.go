// Copyright 2016 The LUCI Authors.
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

package memory

import (
	"sync"
	"time"

	"go.chromium.org/luci/logdog/common/storage/caching"

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
