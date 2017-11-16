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

// Package layered provides a two-layer cache for serializable objects.
package layered

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
	// Marshal converts an item being cached to a byte blob.
	Marshal func(item interface{}) ([]byte, error)
	// Unmarshal takes output of Marshal and converts it to an item to return.
	Unmarshal func(blob []byte) (interface{}, error)
}

// GetOrCreate attempts to grab an item from process or global cache, or create
// it if it's not cached yet.
//
// Fetching an item from the global cache or instantiating a new item happens
// under a per-key lock.
//
// Expiration time is used with seconds precision. Zero expiration time means
// the item doesn't expire on its own.
func (c *Cache) GetOrCreate(ctx context.Context, key string, fn lru.Maker) (interface{}, error) {
	return c.ProcessLRUCache.LRU(ctx).GetOrCreate(ctx, key, func() (interface{}, time.Duration, error) {
		if c.GlobalNamespace == "" {
			panic("empty namespace is forbidden, please specify GlobalNamespace")
		}
		g := caching.GlobalCache(ctx, c.GlobalNamespace)
		if g != nil {
			blob, err := g.Get(ctx, key)
			if err != nil && err != caching.ErrCacheMiss {
				logging.WithError(err).Errorf(ctx, "Failed to read item %q from the global cache", key)
			}
			if err == nil {
				item, expTS, err := c.deserializeItem(blob)
				if err == nil {
					if expTS.IsZero() {
						return item, 0, nil // doesn't have an expiration
					}
					if exp := expTS.Sub(clock.Now(ctx)); exp > 0 {
						return item, exp, nil // not expired yet
					}
					// Otherwise proceed as if we had a cache miss, since the item is
					// expired already.
				} else {
					logging.WithError(err).Errorf(ctx, "Failed to deserialize item %q", key)
				}
			}
		}

		// Either a cache miss or problems with the cached item. Need a new one.
		var expTS time.Time
		item, exp, err := fn()
		switch {
		case err != nil:
			return nil, 0, err
		case exp < 0:
			panic("the expiration time must be non-negative")
		case exp > 0:
			expTS = clock.Now(ctx).Add(exp)
		}

		// Store it in the global cache.
		if g != nil {
			// An error here means the serialization code is buggy, this is a hard
			// failure, unlike an errors from global cache below.
			blob, err := c.serializeItem(item, expTS)
			if err != nil {
				return nil, 0, err
			}
			if err = g.Set(ctx, key, blob, exp); err != nil {
				logging.WithError(err).Errorf(ctx, "Failed to store item %q in the global cache", key)
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
func (c *Cache) serializeItem(item interface{}, expTS time.Time) ([]byte, error) {
	blob, err := c.Marshal(item)
	if err != nil {
		return nil, err
	}

	var deadline uint64
	if !expTS.IsZero() {
		deadline = uint64(expTS.Unix())
	}

	// <version_byte> + <uint64 deadline timestamp> + <blob>
	output := make([]byte, 9+len(blob))
	output[0] = formatVersionByte
	binary.LittleEndian.PutUint64(output[1:], deadline)
	copy(output[9:], blob)
	return output, nil
}

// deserializeItem is reverse of serializeItem.
func (c *Cache) deserializeItem(blob []byte) (item interface{}, expTS time.Time, err error) {
	if len(blob) < 9 {
		err = fmt.Errorf("the received buffer is too small")
		return
	}
	if blob[0] != formatVersionByte {
		err = fmt.Errorf("bad format version, expecting %d, got %d", formatVersionByte, blob[0])
		return
	}
	deadline := binary.LittleEndian.Uint64(blob[1:])
	if deadline != 0 {
		expTS = time.Unix(int64(deadline), 0)
	}
	item, err = c.Unmarshal(blob[9:])
	return
}
