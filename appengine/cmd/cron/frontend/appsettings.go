// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/settings"
)

// appSettings is global application configuration stored in the datastore.
type appSettings struct {
	// ConfigServiceURL is URL of config service to fetch cron definitions from.
	ConfigServiceURL string `json:"config_service_url"`
}

// fetchAppSettings fetches application configuration from the datastore or
// makes default ones.
func fetchAppSettings(c context.Context) (appSettings, error) {
	cfg := appSettings{}
	switch err := settings.Get(c, "cron", &cfg); err {
	case nil:
		return cfg, nil
	case settings.ErrNoSettings:
		storeAppSettings(c, &cfg, "self", "default settings")
		return cfg, nil
	default:
		return cfg, err
	}
}

// storeAppSettings puts app configuration into the datastore.
func storeAppSettings(c context.Context, cfg *appSettings, who, why string) error {
	return settings.Set(c, "cron", cfg, who, why)
}
