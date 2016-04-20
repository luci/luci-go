// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"errors"

	"github.com/golang/protobuf/proto"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/config"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
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

// DevConfig is an auxiliary development configuration. It is only active when
// running under the dev appserver.
type DevConfig struct {
	// ID is the key ID of this configuration.
	ID int64 `gae:"$id"`

	// Config, is set, is the application's text-format configuration protobuf.
	// This is only used when running under dev-appserver.
	Config string `gae:",noindex"`
}

// LoadGlobalConfig loads the global configuration from the AppEngine settings.
//
// If no global configuration is present, settings.ErrNoSettings will be
// returned.
func LoadGlobalConfig(c context.Context) (*GlobalConfig, error) {
	gcfg := GlobalConfig{}
	if err := settings.Get(c, globalConfigSettingsKey, &gcfg); err != nil {
		// If we're running the dev appserver, install some configuration stubs so
		// they can be tweaked locally.
		if err == settings.ErrNoSettings && info.Get(c).IsDevAppServer() {
			log.Infof(c, "Setting up development configuration...")

			if err := gcfg.storeNoValidate(c, "development setup"); err != nil {
				log.WithError(err).Warningf(c, "Failed to install development global config stub.")
			}
			if _, err := getSetupDevConfig(c); err != nil {
				log.WithError(err).Warningf(c, "Failed to install development configuration entry stub.")
			}
		}

		return nil, err
	}
	return &gcfg, nil
}

// Store stores the new global configuration.
func (gcfg *GlobalConfig) Store(c context.Context, why string) error {
	if err := gcfg.Validate(); err != nil {
		return err
	}
	return gcfg.storeNoValidate(c, why)
}

func (gcfg *GlobalConfig) storeNoValidate(c context.Context, why string) error {
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
func (gcfg *GlobalConfig) LoadConfig(c context.Context) (*svcconfig.Config, error) {
	var cfg *config.Config
	if err := gcfg.Validate(); err != nil {
		if !info.Get(c).IsDevAppServer() {
			log.WithError(err).Errorf(c, "Application configuration service is not configured.")
			return nil, settings.ErrNoSettings
		}

		// Dev: Load from DevConfig datastore entry, if present.
		var err error
		cfg, err = getSetupDevConfig(c)
		if err != nil {
			log.WithError(err).Errorf(c, "Failed to load development configuration.")
			return nil, settings.ErrNoSettings
		}
		log.Infof(c, "Using development configuration.")
	} else {
		// Load from remote luci-config service.
		ci := config.Get(c)
		if ci == nil {
			log.Errorf(c, "No configuration instance installed.")
			return nil, settings.ErrNoSettings
		}

		// Load our LUCI-Config values.
		var err error
		cfg, err = ci.GetConfig(gcfg.ConfigSet, gcfg.ConfigPath, false)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"serviceURL": gcfg.ConfigServiceURL,
				"configSet":  gcfg.ConfigSet,
				"configPath": gcfg.ConfigPath,
			}.Errorf(c, "Failed to load config.")
			return nil, settings.ErrNoSettings
		}
	}

	cc := svcconfig.Config{}
	if err := proto.UnmarshalText(cfg.Content, &cc); err != nil {
		log.Fields{
			log.ErrorKey:  err,
			"size":        len(cfg.Content),
			"contentHash": cfg.ContentHash,
			"configSet":   cfg.ConfigSet,
			"revision":    cfg.Revision,
		}.Errorf(c, "Failed to unmarshal configuration protobuf.")
		return nil, settings.ErrNoSettings
	}

	// Validate the configuration.
	if err := validateConfig(&cc); err != nil {
		log.WithError(err).Errorf(c, "Invalid Coordinator configuration.")
		return nil, settings.ErrNoSettings
	}

	return &cc, nil
}

// validateConfig checks the supplied Coordinator config object to ensure that
// it meets a minimum configuration standard expected by our endpoitns and
// handlers.
func validateConfig(cc *svcconfig.Config) error {
	switch {
	case cc == nil:
		return errors.New("configuration is nil")
	case cc.GetCoordinator() == nil:
		return errors.New("no Coordinator configuration")
	default:
		return nil
	}
}

func getSetupDevConfig(c context.Context) (*config.Config, error) {
	dcfg := DevConfig{ID: 1}
	switch err := ds.Get(c).Get(&dcfg); err {
	case nil:
		break

	case ds.ErrNoSuchEntity:
		log.Infof(c, "Installing empty development configuration datastore entry.")
		if err := ds.Get(c).Put(&dcfg); err != nil {
			log.WithError(err).Warningf(c, "Failed to install empty development configuration.")
		}
		return nil, errors.New("no development configuration")

	default:
		return nil, err
	}

	return &config.Config{
		Content: dcfg.Config,
	}, nil
}
