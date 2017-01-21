// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package config

import (
	"time"

	"github.com/luci/luci-go/appengine/datastorecache"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/sync/mutexpool"
	"github.com/luci/luci-go/luci_config/appengine/backend/datastore"
	"github.com/luci/luci-go/luci_config/appengine/gaeconfig"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/caching"

	"golang.org/x/net/context"
)

// CacheOptions is the set of configuration options.
type CacheOptions struct {
	// CacheExpiration is the amount of time that a config entry should be cached.
	//
	// If this value is <= 0, no configuration caching will be enabled. This
	// should not be set for production systems.
	CacheExpiration time.Duration

	// DatastoreCacheAvailable, if true, indicates that requisite services for
	// datastore cache access are installed in the Context. This requires:
	// - A luci/gae datastore service instnace
	// - A settings service instance, presumably gaesettings.
	DatastoreCacheAvailable bool
}

// WrapBackend wraps the supplied base backend in caching layers and returns a
// Conext with the resulting backend installed.
func (o *CacheOptions) WrapBackend(c context.Context, base backend.B) context.Context {
	return backend.WithFactory(c, func(c context.Context) backend.B {
		// Start with our base Backend.
		be := base

		// If our datastore cache is available, fetch settings to see if it's
		// enabled.
		if o.DatastoreCacheAvailable {
			switch s, err := gaeconfig.FetchCachedSettings(c); err {
			case nil:
				// Successfully fetched settings, is our datastore cache enabled?
				switch s.DatastoreCacheMode {
				case gaeconfig.DSCacheEnabled, gaeconfig.DSCacheStrict:
					// Datastore cache is enabled, do we have an expiration configured?
					exp := time.Duration(s.CacheExpirationSec) * time.Second
					if exp > 0 {
						// The cache enabled enable. Install it into our backend.
						locker := &dsCacheLocker{}
						dsCfg := datastore.Config{
							RefreshInterval: exp,
							FailOpen:        s.DatastoreCacheMode == gaeconfig.DSCacheEnabled,
							LockerFunc:      func(context.Context) datastorecache.Locker { return locker },
						}
						be = dsCfg.Backend(be)
					}
				}

			default:
				log.WithError(err).Warningf(c, "Failed to fetch datastore cache settings.")
			}
		}

		// Add a proccache-based config cache.
		if o.CacheExpiration > 0 {
			be = caching.ProcCache(be, o.CacheExpiration)
		}

		return be
	})
}

type dsCacheLocker struct {
	p mutexpool.P
}

func (l *dsCacheLocker) TryWithLock(c context.Context, key string, fn func(context.Context) error) (err error) {
	l.p.WithMutex(key, func() {
		err = fn(c)
	})
	return
}
