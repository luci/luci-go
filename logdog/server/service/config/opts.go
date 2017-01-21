// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package config

import (
	"time"

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
}

// WrapBackend wraps the supplied base backend in caching layers and returns a
// Conext with the resulting backend installed.
func (o *CacheOptions) WrapBackend(c context.Context, base backend.B) context.Context {
	return backend.WithFactory(c, func(c context.Context) backend.B {
		// Start with our base Backend.
		be := base

		// Add a proccache-based config cache.
		if o.CacheExpiration > 0 {
			be = caching.ProcCache(be, o.CacheExpiration)
		}

		return be
	})
}
