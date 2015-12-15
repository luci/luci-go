// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"errors"

	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/settings"
	"golang.org/x/net/context"
)

// globalConfigSettingsKey is the settings key for the Coordinator instance's
// GlobalConfig.
const globalConfigSettingsKey = "LogDogCoordinatorGlobalSettings"

// GlobalConfig is the LogDog Coordinator global configuration.
//
// This is intended to act as an entry point. The majority of the configuration
// will be stored in a "luci-config" service Config protobuf.
type GlobalConfig struct {
	// ConfigServiceURL is the API URL of the base "luci-config" service. If
	// empty, the defualt service URL will be used.
	ConfigServiceURL string `json:"configService"`

	// ConfigSet is the name of the configuration set to load from.
	ConfigSet string `json:"configSet"`
	// ConfigPath is the path of the text-serialized configuration protobuf.
	ConfigPath string `json:"configPath"`
}

// Load loads the global configuration from the AppEngine settings.
func Load(c context.Context) (*GlobalConfig, error) {
	gc := GlobalConfig{}
	if err := settings.Get(c, globalConfigSettingsKey, &gc); err != nil {
		return nil, err
	}
	return &gc, nil
}

// Store stores the new global configuration.
func (gcfg *GlobalConfig) Store(c context.Context, why string) error {
	if err := gcfg.Validate(); err != nil {
		return err
	}

	id := auth.CurrentIdentity(c)
	log.Fields{
		"identity": id,
		"reason":   why,
	}.Infof(c, "Updating global configuration.")
	return settings.Set(c, globalConfigSettingsKey, gcfg, string(id), why)
}

// Validate returns an error if the configuration is not complete and valid.
func (gcfg *GlobalConfig) Validate() error {
	if gcfg.ConfigSet == "" {
		return errors.New("missing config set")
	}
	if gcfg.ConfigPath == "" {
		return errors.New("missing config path")
	}
	return nil
}
