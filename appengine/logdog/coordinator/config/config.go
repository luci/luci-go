// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"errors"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/config"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/services"
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
	ConfigServiceURL string `json:"configService" endpoints:"req"`

	// ConfigSet is the name of the configuration set to load from.
	ConfigSet string `json:"configSet" endpoints:"req"`
	// ConfigPath is the path of the text-serialized configuration protobuf.
	ConfigPath string `json:"configPath" endpoints:"req"`

	// BigTableServiceAccountJSON, if not empty, is the service account JSON file
	// data that will be used for BigTable access.
	//
	// TODO(dnj): Remove this option once Cloud BigTable has cross-project ACLs.
	BigTableServiceAccountJSON []byte `json:"bigTableServiceAccountJson,omitempty"`
}

// LoadGlobalConfig loads the global configuration from the AppEngine settings.
func LoadGlobalConfig(c context.Context) (*GlobalConfig, error) {
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

// LoadConfig loads the services configuration from the current Context.
//
// The Context must have a luci-config interface installed.
func (gcfg *GlobalConfig) LoadConfig(c context.Context) (*services.Config, error) {
	// Load our LUCI-Config values.
	cfg, err := config.Get(c).GetConfig(gcfg.ConfigSet, gcfg.ConfigPath, false)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"serviceURL": gcfg.ConfigServiceURL,
			"configSet":  gcfg.ConfigSet,
			"configPath": gcfg.ConfigPath,
		}.Errorf(c, "Failed to load config.")
		return nil, errors.New("unable to load config")
	}

	cc := services.Config{}
	if err := proto.UnmarshalText(cfg.Content, &cc); err != nil {
		log.Fields{
			log.ErrorKey:  err,
			"size":        len(cfg.Content),
			"contentHash": cfg.ContentHash,
			"configSet":   cfg.ConfigSet,
			"revision":    cfg.Revision,
		}.Errorf(c, "Failed to unmarshal configuration protobuf.")
		return nil, errors.New("configuration is invalid or corrupt")
	}

	return &cc, nil
}

// Load loads the current configuration from "luci-config".
func Load(c context.Context) (*services.Config, error) {
	gcfg, err := LoadGlobalConfig(c)
	if err != nil {
		return nil, err
	}
	return gcfg.LoadConfig(c)
}
