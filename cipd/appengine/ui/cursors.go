// Copyright 2018 The LUCI Authors.
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

package ui

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/caching"
)

type cursorKind string

const instancesListing cursorKind = "v1:instances"

// cursorKey is a cache key for items that link cursor to a previous page.
func (k cursorKind) cursorKey(pkg, cursor string) string {
	blob := sha256.Sum224([]byte(pkg + ":" + cursor))
	return string(k) + ":" + base64.RawStdEncoding.EncodeToString(blob[:])
}

// storePrevCursor stores mapping cursor => prev, so that prev can be fetched
// later by fetchPrevCursor.
//
// Logs and ignores errors. Cursor mapping is non-essential functionality.
func (k cursorKind) storePrevCursor(ctx context.Context, pkg, cursor, prev string) {
	if err := storeInCache(ctx, k.cursorKey(pkg, cursor), []byte(prev)); err != nil {
		logging.Errorf(ctx, "Failed to store prev cursor %q in the cache: %s", k, err)
	}
}

// fetchPrevCursor returns a cursor stored by storePrevCursor.
//
// Logs and ignores errors. Cursor mapping is non-essential functionality.
func (k cursorKind) fetchPrevCursor(ctx context.Context, pkg, cursor string) string {
	blob, err := loadFromCache(ctx, k.cursorKey(pkg, cursor))
	if err != nil {
		logging.Errorf(ctx, "Failed to fetch the prev cursor %q from the cache: %s", k, err)
		return ""
	}
	return string(blob)
}

// localCursorCache is used as a replacement for the global cache when running
// locally during development.
var localCursorCache = caching.RegisterLRUCache[string, string](1)

// storeInCache puts the value in the global memory cache.
//
// If there's no global cache available (e.g. when running locally) uses the
// process cache.
func storeInCache(ctx context.Context, key string, blob []byte) error {
	if global := caching.GlobalCache(ctx, "cursors"); global != nil {
		return global.Set(ctx, key, blob, 24*time.Hour)
	}
	localCursorCache.LRU(ctx).Put(ctx, key, string(blob), time.Hour)
	return nil
}

// loadFromCache loads the value previously stored with storeInCache.
//
// If there's no such value, returns (nil, nil).
func loadFromCache(ctx context.Context, key string) ([]byte, error) {
	if global := caching.GlobalCache(ctx, "cursors"); global != nil {
		val, err := global.Get(ctx, key)
		if err == caching.ErrCacheMiss {
			return nil, nil
		}
		return val, err
	}
	if val, ok := localCursorCache.LRU(ctx).Get(ctx, key); ok {
		return []byte(val), nil
	}
	return nil, nil
}
