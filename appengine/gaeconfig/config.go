// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gaeconfig

import (
	"fmt"
	"time"

	mc "github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/filters/caching"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// AddCacheFilter adds a memcache-caching config filter to the context.
//
// A Transport from "gaeauth/client" capable of talking to the underlying
// service should be installed prior to calling this method.
func AddCacheFilter(c context.Context, expire time.Duration) context.Context {
	return config.AddFilters(c, func(ic context.Context, cfg config.Interface) config.Interface {
		return NewCacheFilter(ic, expire)(ic, cfg)
	})
}

// NewCacheFilter returns memcache-caching config filter.
func NewCacheFilter(c context.Context, expire time.Duration) config.Filter {
	o := caching.Options{
		Cache: &memCache{
			Context: c,
		},
		Expiration: expire,
	}
	return caching.NewFilter(o)
}

type memCache struct {
	Context context.Context
}

func (c *memCache) Store(ctx context.Context, key string, expire time.Duration, value []byte) {
	mi := mc.Get(ctx)
	if err := mi.Set(mi.NewItem(c.key(key)).SetExpiration(expire).SetValue(value)); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"key":        key,
			"expire":     expire,
		}.Warningf(ctx, "Failed to store cache value.")
	}
}

func (c *memCache) Retrieve(ctx context.Context, key string) []byte {
	item, err := mc.Get(ctx).Get(c.key(key))
	if err != nil {
		if err != mc.ErrCacheMiss {
			log.Fields{
				log.ErrorKey: err,
				"key":        key,
			}.Warningf(ctx, "Failed to retrieve cache value.")
		}
		return nil
	}

	return item.Value()
}

func (c *memCache) key(base string) string {
	return fmt.Sprintf("luci-config:%s", base)
}
