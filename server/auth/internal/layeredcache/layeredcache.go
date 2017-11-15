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

package layeredcache

import (
	"encoding/binary"
	"fmt"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/caching"
)

// Cache implements a cache of serializable objects on top of process and
// global caches.
//
// If the global cache is not available or fails, degrades to using only process
// cache.
//
// Since global cache errors are ignored, gives no guarantees of consistency or
// item uniqueness. Thus supposed to be used only when caching results of
// computations without side effects.
type Cache struct {
	// ProcessLRUCache is a handle to a process LRU cache that holds the data.
	ProcessLRUCache caching.LRUHandle
	// GlobalNamespace is a global cache namespace to use for the data.
	GlobalNamespace string
	// KeyMapper takes a key passed to GetOrCreate and returns a global cache key.
	KeyMapper func(key interface{}) (string, error)
	// Marshal converts an item being cached to a byte blob.
	Marshal func(item interface{}) ([]byte, error)
	// Unmarshal takes output of Marshal and converts it to an item to return.
	Unmarshal func(blob []byte) (interface{}, error)
}

// GetOrCreate attempts to grab an item from process or global cache, or create
// it if it's not cached yet.
//
// Fetching an item from the global cache or instantiating a new item happens
// under a per-item lock.
//
// Expiration time is used with seconds precision. Zero expiration time means
// the item doesn't expire on its own.
func (c *Cache) GetOrCreate(ctx context.Context, key interface{}, fn lru.Maker) (interface{}, error) {
	return c.ProcessLRUCache.LRU(ctx).GetOrCreate(ctx, key, func() (interface{}, time.Duration, error) {
		k, err := c.KeyMapper(key)
		if err != nil {
			return nil, 0, err
		}

		g := caching.GlobalCache(ctx, c.GlobalNamespace)
		if g != nil {
			blob, err := g.Get(ctx, k)
			if err != nil && err != caching.ErrCacheMiss {
				logging.WithError(err).Errorf(ctx, "Failed to read item %q from the global cache", k)
			}
			if blob != nil {
				item, exp, err := c.deserializeItem(ctx, blob)
				if err == nil {
					// Note: this would also put it in the process cache.
					return item, exp, nil
				}
				if err != caching.ErrCacheMiss {
					logging.WithError(err).Errorf(ctx, "Failed to deserialize item %q", k)
				}
			}
		}

		// Either a cache miss or problems with the cached item. Need a new one.
		item, exp, err := fn()
		switch {
		case err != nil:
			return nil, 0, err
		case exp < 0:
			panic("the expiration time must be non-negative")
		}

		// Store it in the global cache.
		if g != nil {
			// An error here means the serialization code is buggy, this is a hard
			// failure, unlike an errors from global cache below.
			blob, err := c.serializeItem(ctx, item, exp)
			if err != nil {
				return nil, 0, err
			}
			if err = g.Set(ctx, k, blob, exp); err != nil {
				logging.WithError(err).Errorf(ctx, "Failed to store item %q in the global cache", k)
			}
		}

		return item, exp, nil
	})
}

// formatVersionByte indicates what serialization format is used, it is stored
// as a first byte of the serialized data.
//
// Serialized items with different value of the first byte are rejected.
const formatVersionByte = 1

// serializeItem packs item and its expiration time into a byte blob.
func (c *Cache) serializeItem(ctx context.Context, item interface{}, exp time.Duration) ([]byte, error) {
	blob, err := c.Marshal(item)
	if err != nil {
		return nil, err
	}

	var deadline uint64
	if exp != 0 {
		deadline = uint64(clock.Now(ctx).Add(exp).Unix())
	}

	// <version_byte> + <uint64 deadline timestamp> + <blob>
	output := make([]byte, 9+len(blob))
	output[0] = formatVersionByte
	binary.LittleEndian.PutUint64(output[1:], deadline)
	copy(output[9:], blob)
	return output, nil
}

// deserializeItem is reverse of serializeItem.
//
// It returns caching.ErrCacheMiss if the deserialized item is actually expired
// already.
func (c *Cache) deserializeItem(ctx context.Context, blob []byte) (item interface{}, exp time.Duration, err error) {
	if len(blob) < 9 {
		return nil, 0, fmt.Errorf("the received buffer is too small")
	}
	if blob[0] != formatVersionByte {
		return nil, 0, fmt.Errorf("bad format version, expecting %d, got %d", formatVersionByte, blob[0])
	}
	deadline := binary.LittleEndian.Uint64(blob[1:])
	if deadline != 0 {
		exp = time.Unix(int64(deadline), 0).Sub(clock.Now(ctx))
		if exp <= 0 {
			return nil, 0, caching.ErrCacheMiss
		}
	}
	item, err = c.Unmarshal(blob[9:])
	return
}
