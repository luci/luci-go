// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package openid

import (
	"errors"

	"github.com/luci/luci-go/server/settings"
	"golang.org/x/net/context"
)

// SettingsKey is key for OpenID settings (described by Settings struct) in
// settings store. See github.com/luci/luci-go/server/settings.
const SettingsKey = "openid_auth"

var (
	// ErrNotConfigured is returned by various functions if OpenID settings are
	// not properly configured.
	ErrNotConfigured = errors.New("openid: not configured")
)

// Settings contain parameters of OpenID protocol. They are stored in app
// settings store under SettingsKey key.
type Settings struct {
	// DiscoveryURL is where to grab discovery document with provider's config.
	// Use `https://accounts.google.com/.well-known/openid-configuration` for
	// Google OpenID Connect provider.
	DiscoveryURL string `json:"discovery_url"`

	// ClientID identifies OAuth2 Web client representing the application. Create
	// one in Cloud Console if using Google OpenID Connect provider.
	ClientID string `json:"client_id"`

	// ClientSecret is a secret associated with ClientID.
	ClientSecret string `json:"client_secret"`

	// RedirectURI must be `https://<apphost>/auth/openid/callback`. It is stored
	// in config explicitly to remind admin that OAuth2 client in Cloud Console
	// must be configured accordingly.
	RedirectURI string `json:"redirect_uri"`
}

// fetchCachedSettings fetches OpenID configuration from the settings store
// or puts a default one there.
func fetchCachedSettings(c context.Context) (*Settings, error) {
	cfg := &Settings{}
	if err := settings.Get(c, SettingsKey, cfg); err != settings.ErrNoSettings {
		return cfg, err
	}
	settings.Set(c, SettingsKey, cfg, "self", "default OpenID settings")
	return cfg, nil
}
