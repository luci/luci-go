// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"net/url"

	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/logdog/api/config/svcconfig"

	"golang.org/x/net/context"
)

var (
	// ErrInvalidConfig is returned when the configuration exists, but is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")
)

// Config is the LogDog Coordinator service configuration.
type Config struct {
	svcconfig.Config
	// Settings are per-instance settings.
	Settings Settings

	// ConfigServiceURL is the config service's URL.
	ConfigServiceURL url.URL `json:"-"`
	// ConfigSet is the name of the service config set that is being used.
	ConfigSet config.Set `json:"-"`
	// ServiceConfigPath is the path within ConfigSet of the service
	// configuration.
	ServiceConfigPath string `json:"-"`
}

// ServiceConfigPath returns the config set and path for this application's
// service configuration.
func ServiceConfigPath(c context.Context) (config.Set, string) {
	return cfgclient.CurrentServiceConfigSet(c), svcconfig.ServiceConfigPath
}

// Load loads the service configuration. This includes:
//	- The config service settings.
//	- The service configuration, loaded from the config service.
//	- Additional Settings data from datastore via settings.
//
// The service config is minimally validated prior to being returned.
func Load(c context.Context) (*Config, error) {
	// Unmarshal the config into service configuration.
	cfg := Config{
		ConfigServiceURL: cfgclient.ServiceURL(c),
	}
	cfg.ConfigSet, cfg.ServiceConfigPath = ServiceConfigPath(c)

	// Load our service-level config.
	if err := cfgclient.Get(c, cfgclient.AsService, cfg.ConfigSet, cfg.ServiceConfigPath,
		textproto.Message(&cfg.Config), nil); err != nil {

		log.Fields{
			log.ErrorKey: err,
			"configSet":  cfg.ConfigSet,
			"configPath": cfg.ServiceConfigPath,
		}.Errorf(c, "Failed to load configuration from config service.")
		return nil, err
	}

	// Validate the configuration.
	if err := validateServiceConfig(&cfg.Config); err != nil {
		log.WithError(err).Errorf(c, "Invalid Coordinator configuration.")
		return nil, ErrInvalidConfig
	}

	// Load our settings.
	if err := cfg.Settings.Load(c); err != nil {
		log.WithError(err).Errorf(c, "Failed to load settings.")
		return nil, ErrInvalidConfig
	}

	return &cfg, nil
}

// validateServiceConfig checks the supplied service config object to ensure
// that it meets a minimum configuration standard expected by our endpoitns and
// handlers.
func validateServiceConfig(cc *svcconfig.Config) error {
	switch {
	case cc == nil:
		return errors.New("configuration is nil")
	case cc.GetCoordinator() == nil:
		return errors.New("no Coordinator configuration")
	default:
		return nil
	}
}
