// Copyright 2017 The LUCI Authors.
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
	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/textproto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/machine-db/api/config/v1"
)

// platformsConfig is the name of the config file enumerating platforms.
const platformsConfig = "platforms.cfg"

// importPlatformConfigs fetches, validates, and applies platform configs.
func importPlatformConfigs(c context.Context, configSet cfgtypes.ConfigSet) error {
	platform := &config.PlatformsConfig{}
	metadata := &cfgclient.Meta{}
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, platformsConfig, textproto.Message(platform), metadata); err != nil {
		return errors.Annotate(err, "failed to load config: %s", platform).Err()
	}
	logging.Infof(c, "Found config revision: %s", metadata.Revision)

	validationContext := &validation.Context{Context: c}
	validatePlatformsConfig(validationContext, platform)
	if err := validationContext.Finalize(); err != nil {
		return errors.Annotate(err, "failed to validate config").Err()
	}
	return nil
}

// validatePlatformsConfig validates platforms.cfg.
func validatePlatformsConfig(c *validation.Context, cfg *config.PlatformsConfig) error {
	// Platforem names must be unique.
	// Keep records of ones we've already seen.
	platforms := stringset.New(len(cfg.Platform))

	c.SetFile(platformsConfig)
	for _, platformConfig := range cfg.Platform {
		if platformConfig.Name == "" {
			c.Errorf("platform names are required and must be non-empty")
		}
		if platforms.Has(platformConfig.Name) {
			c.Errorf("duplicate platform %q", platformConfig.Name)
		}
		platforms.Add(platformConfig.Name)
	}
	return nil
}
