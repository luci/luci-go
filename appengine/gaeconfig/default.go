// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gaeconfig

import (
	"errors"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/remote"
)

// ErrNotConfigured is returned by New if config service URL is not set. Usually
// happens for new apps.
var ErrNotConfigured = errors.New("config service URL is not set in settings")

// New constructs default luci-config client.
//
// The client is configured to use luci-config URL specified in the settings,
// using GAE app service account for authentication.
//
// Returns ErrNotConfigured if luci config URL is not set.
func New(c context.Context) (config.Interface, error) {
	settings, err := FetchCachedSettings(c)
	if err != nil {
		return nil, err
	}

	if settings.ConfigServiceURL == "" {
		return nil, ErrNotConfigured
	}

	c = client.UseServiceAccountTransport(c, nil, nil)
	cfg := remote.New(c, settings.ConfigServiceURL+"/_ah/api/config/v1/")
	if settings.CacheExpirationSec != 0 {
		f := NewCacheFilter(c, time.Duration(settings.CacheExpirationSec)*time.Second)
		cfg = f(c, cfg)
	}
	return cfg, nil
}
