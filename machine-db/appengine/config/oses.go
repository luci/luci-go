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

// osesConfig is the name of the config file enumerating operating systems.
const osesConfig = "oses.cfg"

// importOperatingSystemConfigs fetches, validates, and applies operating system configs.
func importOperatingSystemConfigs(c context.Context, configSet cfgtypes.ConfigSet) error {
	os := &config.OperatingSystemsConfig{}
	metadata := &cfgclient.Meta{}
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, osesConfig, textproto.Message(os), metadata); err != nil {
		return errors.Annotate(err, "failed to load config: %s", os).Err()
	}
	logging.Infof(c, "Found config revision: %s", metadata.Revision)

	validationContext := &validation.Context{Context: c}
	validateOperatingSystemsConfig(validationContext, os)
	if err := validationContext.Finalize(); err != nil {
		return errors.Annotate(err, "failed to validate config").Err()
	}
	return nil
}

// validateOperatingSystemsConfig validates oses.cfg.
func validateOperatingSystemsConfig(c *validation.Context, cfg *config.OperatingSystemsConfig) error {
	// Operating system names must be unique.
	// Keep records of ones we've already seen.
	oses := stringset.New(len(cfg.OperatingSystem))

	c.SetFile(osesConfig)
	for _, osConfig := range cfg.OperatingSystem {
		if osConfig.Name == "" {
			c.Errorf("operating system names are required and must be non-empty")
		}
		if oses.Has(osConfig.Name) {
			c.Errorf("duplicate operating system %q", osConfig.Name)
		}
		oses.Add(osConfig.Name)
	}
	return nil
}
