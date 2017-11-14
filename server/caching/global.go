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
	"errors"
	"time"

	"golang.org/x/net/context"
)

// ErrCacheMiss is returned by BlobCache.Get if the requested item is missing.
var ErrCacheMiss = errors.New("cache miss")

// BlobCache is a minimal interface for a cross-process memcache-like cache.
//
// It exists to support low-level server libraries that target all sorts of
// environments that may have different memcache implementations (or none at
// all).
//
// This interface provides the smallest sufficient API to support needs of
// low-level libraries. If you need anything fancier, consider using the
// concrete memcache implementation directly (e.g. use luci/gae's memcache
// library when running on GAE Standard).
//
// BlobCache has the following properties:
//   * All service processes share this cache (thus 'global').
//   * The global cache is namespaced (see GlobalCache function below).
//   * It is strongly consistent.
//   * Items can be evicted at random times.
//   * Key size is <250 bytes.
//   * Item size is <1 Mb.
type BlobCache interface {
	// Get returns a cached item or ErrCacheMiss if it's not in the cache.
	Get(c context.Context, key string) ([]byte, error)

	// Set unconditionally overwrites an item in the cache.
	//
	// If 'exp' is zero, the item will have no expiration time.
	Set(c context.Context, key string, value []byte, exp time.Duration) error
}

// BlobCacheProvider returns a BlobCache instance targeting a namespace.
type BlobCacheProvider func(namespace string) BlobCache

var globalCacheKey = "server.caching Global Cache"

// WithGlobalCache installs a global cache implementation into the supplied
// context.
func WithGlobalCache(c context.Context, provider BlobCacheProvider) context.Context {
	return context.WithValue(c, &globalCacheKey, provider)
}

// GlobalCache returns a global cache targeting the given namespace or nil if
// the global cache is not available in the current environment.
//
// Libraries that can live without a global cache mush handle its absence
// gracefully (e.g by falling back to using only the process cache).
func GlobalCache(c context.Context, namespace string) BlobCache {
	if p, _ := c.Value(&globalCacheKey).(BlobCacheProvider); p != nil {
		return p(namespace)
	}
	return nil
}
