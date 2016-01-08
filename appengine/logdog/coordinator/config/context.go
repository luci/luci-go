// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	gaeauthClient "github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/appengine/gaeconfig"
	"github.com/luci/luci-go/common/config/impl/remote"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/settings"
	"golang.org/x/net/context"
)

// WithConfig sets up luci-config as a remote configuration source based on the
// current GlobalConfig.
//
// If the GlobalConfig doesn't fully specify a luci-config source, no
// luci-config will be installed.
func WithConfig(c context.Context) context.Context {
	gcfg, err := LoadGlobalConfig(c)
	switch err {
	case nil:
		// Install remote "luci-config" service.
		//
		// Use an e-mail OAuth2-authenticated transport to pull from "luci-config".
		c = gaeauthClient.UseServiceAccountTransport(c, nil, nil)
		c = remote.Use(c, gcfg.ConfigServiceURL)

		// Add a memcache-based caching filter.
		c = gaeconfig.AddFilter(c, gaeconfig.DefaultExpire)

	case settings.ErrNoSettings:
		// No settings, so no configuration will be installed.
		log.Warningf(c, "No luci-config endpoint is set.")

	default:
		log.WithError(err).Errorf(c, "Failed to load application configuration.")
	}

	return c
}
