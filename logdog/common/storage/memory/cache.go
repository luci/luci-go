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
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/logdog/common/storage"
)

// Cache is an in-memory storage.Cache implementation.
type Cache struct {
	mu       sync.Mutex
	cacheMap map[storage.CacheKey][]byte
}

var _ storage.Cache = (*Cache)(nil)

// Put implements storage.Cache.
func (c *Cache) Put(ctx context.Context, key storage.CacheKey, val []byte, exp time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cacheMap == nil {
		c.cacheMap = make(map[storage.CacheKey][]byte)
	}

	c.cacheMap[key] = val
}

// Get implements storage.Cache.
func (c *Cache) Get(ctx context.Context, key storage.CacheKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.cacheMap[key]
	return v, ok
}
