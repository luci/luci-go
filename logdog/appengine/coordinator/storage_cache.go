// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package coordinator

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/logdog/common/storage/caching"

	"github.com/luci/gae/service/memcache"

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
func (sc *StorageCache) Get(c context.Context, items ...*caching.Item) {
	mcItems := make([]memcache.Item, len(items))
	for i, itm := range items {
		mcItems[i] = memcache.NewItem(c, sc.mkCacheKey(itm))
	}

	err := memcache.Get(c, mcItems...)
	sc.memcacheErrCB(err, len(mcItems), func(err error, i int) {
		// By default, no data.
		items[i].Data = nil

		switch err {
		case nil:
			itemData := mcItems[i].Value()
			if len(itemData) == 0 {
				log.Warningf(c, "Cached storage missing compression byte.")
				return
			}
			isCompressed, itemData := itemData[0], itemData[1:]

			if isCompressed != 0x00 {
				// This entry is compressed.
				zr, err := zlib.NewReader(bytes.NewReader(itemData))
				if err != nil {
					log.Fields{
						log.ErrorKey: err,
						"key":        mcItems[i].Key(),
					}.Warningf(c, "Failed to create ZLIB reader.")
					return
				}
				defer zr.Close()

				if itemData, err = ioutil.ReadAll(zr); err != nil {
					log.Fields{
						log.ErrorKey: err,
						"key":        mcItems[i].Key(),
					}.Warningf(c, "Failed to decompress cached item.")
					return
				}
			}
			items[i].Data = itemData

		case memcache.ErrCacheMiss:
			break

		default:
			log.Fields{
				log.ErrorKey: err,
				"key":        mcItems[i].Key(),
			}.Warningf(c, "Error retrieving cached entry.")
		}
	})
}

// Put implements caching.Cache.
func (sc *StorageCache) Put(c context.Context, exp time.Duration, items ...*caching.Item) {
	threshold := sc.compressionThreshold
	if threshold == 0 {
		threshold = defaultCompressionThreshold
	}

	var (
		buf    bytes.Buffer
		zw     zlib.Writer
		usedZW bool
	)
	defer func() {
		if usedZW {
			zw.Close()
		}
	}()

	mcItems := make([]memcache.Item, 0, len(items))
	for _, itm := range items {
		if itm.Data == nil {
			continue
		}

		// Compress the data in the cache item.
		writeItemData := func(d []byte) bool {
			buf.Reset()
			buf.Grow(len(d) + 1)

			if len(d) < threshold {
				// Do not compress the item. Write a "0x00" to indicate that it is
				// not compressed.
				if err := buf.WriteByte(0x00); err != nil {
					log.WithError(err).Warningf(c, "Failed to write compression byte.")
					return false
				}
				if _, err := buf.Write(d); err != nil {
					log.WithError(err).Warningf(c, "Failed to write storage cache data.")
					return false
				}
				return true
			}

			// Compress the item. Write a "0x01" to indicate that it is compressed.
			zw := zlib.NewWriter(&buf)
			if err := buf.WriteByte(0x01); err != nil {
				log.WithError(err).Warningf(c, "Failed to write compression byte.")
				return false
			}
			defer zw.Close()

			if _, err := zw.Write(d); err != nil {
				log.WithError(err).Warningf(c, "Failed to compress storage cache data.")
				return false
			}
			if err := zw.Flush(); err != nil {
				log.WithError(err).Warningf(c, "Failed to flush compressed storage cache data.")
				return false
			}
			return true
		}

		if !writeItemData(itm.Data) {
			continue
		}

		mcItem := memcache.NewItem(c, sc.mkCacheKey(itm))
		mcItem.SetValue(append([]byte(nil), buf.Bytes()...))
		if exp > 0 {
			mcItem.SetExpiration(exp)
		}
		mcItems = append(mcItems, mcItem)
	}

	err := memcache.Set(c, mcItems...)
	sc.memcacheErrCB(err, len(mcItems), func(err error, i int) {
		switch err {
		case nil, memcache.ErrNotStored:
			break

		default:
			log.Fields{
				log.ErrorKey: err,
				"key":        mcItems[i].Key(),
			}.Warningf(c, "Error storing cached entry.")
		}
	})
}

func (*StorageCache) memcacheErrCB(err error, count int, cb func(error, int)) {
	merr, _ := err.(errors.MultiError)
	if merr != nil && len(merr) != count {
		panic(fmt.Errorf("MultiError count mismatch (%d != %d)", len(merr), count))
	}

	for i := 0; i < count; i++ {
		switch {
		case err == nil:
			cb(nil, i)

		case merr == nil:
			cb(err, i)

		default:
			cb(merr[i], i)
		}
	}
}

func (*StorageCache) mkCacheKey(itm *caching.Item) string {
	return strings.Join([]string{
		"storage_cache",
		schemaVersion,
		itm.Schema,
		itm.Type,
		itm.Key,
	}, "_")
}
