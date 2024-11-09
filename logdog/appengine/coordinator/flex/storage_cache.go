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

package flex

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"time"

	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/logdog/common/storage"
)

const (
	// defaultCompressionThreshold is the threshold where entries will become
	// compressed. If the size of data exceeds this threshold, it will be
	// compressed with zlib in the cache.
	defaultCompressionThreshold = 64 * 1024 // 64KiB
)

var storageCache = caching.RegisterLRUCache[storage.CacheKey, []byte](65535)

// StorageCache implements a generic storage.Cache for Storage instances.
//
// This uses the process cache for the underlying cache.
type StorageCache struct {
	compressionThreshold int
}

// Get implements storage.Cache.
func (sc *StorageCache) Get(c context.Context, key storage.CacheKey) ([]byte, bool) {
	data, ok := storageCache.LRU(c).Get(c, key)
	if !ok {
		return nil, false
	}

	if len(data) == 0 {
		// No cache item (missing or invalid/empty).
		return nil, false
	}

	isCompressed, data := data[0], data[1:]
	if isCompressed != 0x00 {
		// This entry is compressed.
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"key":        key,
			}.Warningf(c, "Failed to create ZLIB reader.")
			return nil, false
		}
		defer zr.Close()

		if data, err = io.ReadAll(zr); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"key":        key,
			}.Warningf(c, "Failed to decompress cached item.")
			return nil, false
		}
	}
	return data, true
}

// Put implements storage.Cache.
func (sc *StorageCache) Put(c context.Context, key storage.CacheKey, data []byte, exp time.Duration) {
	ct := sc.compressionThreshold
	if ct <= 0 {
		ct = defaultCompressionThreshold
	}

	var buf bytes.Buffer
	buf.Grow(len(data) + 1)

	if len(data) < ct {
		// Do not compress the item. Write a "0x00" to indicate that it is
		// not compressed.
		if err := buf.WriteByte(0x00); err != nil {
			log.WithError(err).Warningf(c, "Failed to write compression byte.")
			return
		}
		if _, err := buf.Write(data); err != nil {
			log.WithError(err).Warningf(c, "Failed to write storage cache data.")
			return
		}
	} else {
		// Compress the item. Write a "0x01" to indicate that it is compressed.
		if err := buf.WriteByte(0x01); err != nil {
			log.WithError(err).Warningf(c, "Failed to write compression byte.")
			return
		}

		zw := zlib.NewWriter(&buf)
		if _, err := zw.Write(data); err != nil {
			log.WithError(err).Warningf(c, "Failed to compress storage cache data.")
			zw.Close()
			return
		}
		if err := zw.Close(); err != nil {
			log.WithError(err).Warningf(c, "Failed to close compressed storage cache data.")
			return
		}
	}

	storageCache.LRU(c).Put(c, key, buf.Bytes(), exp)
}
