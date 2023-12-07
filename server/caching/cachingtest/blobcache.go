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

// Package cachingtest contains helpers for testing code that uses caching
// package.
package cachingtest

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/data/caching/lru"

	"go.chromium.org/luci/server/caching"
)

// BlobCache implements caching.BlobCache on top of lru.Cache for testing.
//
// Useful for mocking caching.GlobalCache in tests. See also WithGlobalCache
// below.
type BlobCache struct {
	LRU *lru.Cache[string, []byte] // the underlying LRU cache
	Err error                      // if non-nil, will be returned by Get and Set
}

// NewBlobCache initializes empty blob cache with unlimited capacity.
func NewBlobCache() *BlobCache {
	return &BlobCache{LRU: lru.New[string, []byte](0)}
}

// Get returns a cached item or ErrCacheMiss if it's not in the cache.
func (b *BlobCache) Get(c context.Context, key string) ([]byte, error) {
	if b.Err != nil {
		return nil, b.Err
	}
	item, ok := b.LRU.Get(c, key)
	if !ok {
		return nil, caching.ErrCacheMiss
	}
	return item, nil
}

// Set unconditionally overwrites an item in the cache.
func (b *BlobCache) Set(c context.Context, key string, value []byte, exp time.Duration) error {
	if b.Err != nil {
		return b.Err
	}
	b.LRU.Put(c, key, value, exp)
	return nil
}

// WithGlobalCache installs given BlobCaches as "global" in the context.
//
// 'caches' is a map from a namespace to BlobCache instance. If some other
// namespace is requested, the corresponding caching.GlobalCache call will
// panic.
func WithGlobalCache(c context.Context, caches map[string]caching.BlobCache) context.Context {
	return caching.WithGlobalCache(c, func(ns string) caching.BlobCache {
		b, ok := caches[ns]
		if !ok {
			panic(fmt.Errorf("using unexpected global namespace %q", ns))
		}
		return b
	})
}
