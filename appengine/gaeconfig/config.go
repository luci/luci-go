// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gaeconfig

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/filters/caching"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/proccache"
	"golang.org/x/net/context"
)

// AddCacheFilter adds a proccache-and-memcache-caching config filter to the context.
//
// A Transport from "gaeauth/client" capable of talking to the underlying
// service should be installed prior to calling this method.
func AddCacheFilter(c context.Context, expire time.Duration) context.Context {
	return config.AddFilters(c, func(ic context.Context, cfg config.Interface) config.Interface {
		return NewCacheFilter(ic, expire)(ic, cfg)
	})
}

// NewCacheFilter returns proccache-and-memcache-caching config filter.
func NewCacheFilter(c context.Context, expire time.Duration) config.Filter {
	o := caching.Options{
		Cache:      &cache{},
		Expiration: expire,
	}
	return caching.NewFilter(o)
}

type cache struct{}

type proccacheKey string

func (c *cache) Store(ctx context.Context, baseKey string, expire time.Duration, value []byte) {
	k := cacheKey(baseKey)

	proccache.Put(ctx, k, value, expire)

	// value in memcache is [varint(expiration_ts.Millis) ++ value]
	// value in proccache is [value]
	//
	// This is because memcache doesn't populate the .Expiration field of the
	// memcache Item on Get operations :(
	stamp := datastore.TimeToInt(clock.Now(ctx).UTC().Add(expire))
	buf := make([]byte, binary.MaxVarintLen64)
	value = append(buf[:binary.PutVarint(buf, stamp)], value...)

	mc := memcache.Get(ctx)
	itm := mc.NewItem(string(k)).SetExpiration(expire).SetValue(value)
	if err := mc.Set(itm); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"key":        baseKey,
			"expire":     expire,
		}.Warningf(ctx, "Failed to store cache value.")
	}
}

func (c *cache) Retrieve(ctx context.Context, baseKey string) []byte {
	k := cacheKey(baseKey)
	ret, err := proccache.GetOrMake(ctx, k, func() (value interface{}, exp time.Duration, err error) {
		item, err := memcache.Get(ctx).Get(string(k))
		if err != nil {
			if err != memcache.ErrCacheMiss {
				log.Fields{
					log.ErrorKey: err,
					"key":        baseKey,
				}.Warningf(ctx, "Failed to retrieve memcache value.")
			}
			return
		}

		buf := bytes.NewBuffer(item.Value())
		expStamp, err := binary.ReadVarint(buf)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"key":        baseKey,
			}.Warningf(ctx, "Failed to decode stamp in memcache value.")
			return
		}

		// proccache will ignore this value if exp is in the past
		exp = datastore.IntToTime(expStamp).Sub(clock.Now(ctx))
		value = buf.Bytes()
		return
	})
	if err != nil {
		return nil
	}
	return ret.([]byte)
}

func cacheKey(baseKey string) proccacheKey {
	return proccacheKey(fmt.Sprintf("luci-config:v2:%s", baseKey))
}
