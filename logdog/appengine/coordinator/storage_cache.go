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

package coordinator

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"
	"strings"
	"time"

	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/common/storage"

	"go.chromium.org/gae/service/memcache"

	"golang.org/x/net/context"
)

const (
	schemaVersion = "v1"

	// defaultCompressionThreshold is the threshold where entries will become
	// compressed. If the size of data exceeds this threshold, it will be
	// compressed with zlib in the cache.
	defaultCompressionThreshold = 64 * 1024 // 64KiB
)

// StorageCache implements a generic caching.Cache for Storage instances.
type StorageCache struct {
	compressionThreshold int
}

// Get implements caching.Cache.
func (sc *StorageCache) Get(c context.Context, key storage.CacheKey) ([]byte, bool) {
	mcItem := memcache.NewItem(c, sc.mkCacheKey(key))

	switch err := memcache.Get(c, mcItem); err {
	case nil:
		itemData := mcItem.Value()
		if len(itemData) == 0 {
			log.Warningf(c, "Cached storage missing compression byte.")
			return nil, false
		}

		isCompressed, itemData := itemData[0], itemData[1:]

		if isCompressed != 0x00 {
			// This entry is compressed.
			zr, err := zlib.NewReader(bytes.NewReader(itemData))
			if err != nil {
				log.Fields{
					log.ErrorKey: err,
					"key":        mcItem.Key(),
				}.Warningf(c, "Failed to create ZLIB reader.")
				return nil, false
			}
			defer zr.Close()

			if itemData, err = ioutil.ReadAll(zr); err != nil {
				log.Fields{
					log.ErrorKey: err,
					"key":        mcItem.Key(),
				}.Warningf(c, "Failed to decompress cached item.")
				return nil, false
			}
		}
		return itemData, true

	case memcache.ErrCacheMiss:
		return nil, false

	default:
		log.Fields{
			log.ErrorKey: err,
			"key":        mcItem.Key(),
		}.Warningf(c, "Error retrieving cached entry.")
		return nil, false
	}
}

// Put implements caching.Cache.
func (sc *StorageCache) Put(c context.Context, key storage.CacheKey, val []byte, exp time.Duration) {
	threshold := sc.compressionThreshold
	if threshold == 0 {
		threshold = defaultCompressionThreshold
	}

	// Compress the data in the cache item.
	var buf bytes.Buffer
	buf.Grow(len(val) + 1)

	if len(val) < threshold {
		// Do not compress the item. Write a "0x00" to indicate that it is
		// not compressed.
		if err := buf.WriteByte(0x00); err != nil {
			log.WithError(err).Warningf(c, "Failed to write compression byte.")
			return
		}
		if _, err := buf.Write(val); err != nil {
			log.WithError(err).Warningf(c, "Failed to write storage cache data.")
			return
		}
	} else {
		// Compress the item. Write a "0x01" to indicate that it is compressed.
		zw := zlib.NewWriter(&buf)
		if err := buf.WriteByte(0x01); err != nil {
			log.WithError(err).Warningf(c, "Failed to write compression byte.")
			return
		}

		if _, err := zw.Write(val); err != nil {
			zw.Close()
			log.WithError(err).Warningf(c, "Failed to compress storage cache data.")
			return
		}
		if err := zw.Close(); err != nil {
			log.WithError(err).Warningf(c, "Failed to flush compressed storage cache data.")
			return
		}
	}

	mcItem := memcache.NewItem(c, sc.mkCacheKey(key))
	mcItem.SetValue(buf.Bytes())
	if exp > 0 {
		mcItem.SetExpiration(exp)
	}

	switch err := memcache.Set(c, mcItem); err {
	case nil, memcache.ErrNotStored:
	default:
		log.Fields{
			log.ErrorKey: err,
			"key":        mcItem.Key(),
		}.Warningf(c, "Error storing cached entry.")
	}
}

func (*StorageCache) mkCacheKey(key storage.CacheKey) string {
	return strings.Join([]string{
		"storage_cache",
		schemaVersion,
		key.Schema,
		key.Type,
		key.Key,
	}, "_")
}
