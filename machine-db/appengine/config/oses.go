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
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/textproto"

	"go.chromium.org/luci/machine-db/api/config/v1"
)

// osesConfig is the name of the config file enumerating operating systems.
const osesConfig = "oses.cfg"

// importOSConfigs fetches, validates, and applies operating system configs.
func importOSConfigs(c context.Context, configSet cfgtypes.ConfigSet) error {
	os := &config.OSesConfig{}
	metadata := &cfgclient.Meta{}
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, osesConfig, textproto.Message(os), metadata); err != nil {
		return errors.Annotate(err, "failed to load %s", osesConfig).Err()
	}
	logging.Infof(c, "Found %s revision %q", osesConfig, metadata.Revision)

	validationContext := &validation.Context{Context: c}
	validationContext.SetFile(osesConfig)
	validateOSesConfig(validationContext, os)
	if err := validationContext.Finalize(); err != nil {
		return errors.Annotate(err, "invalid config").Err()
	}
	return nil
}

// validateOSesConfig validates oses.cfg.
func validateOSesConfig(c *validation.Context, cfg *config.OSesConfig) {
	// Operating system names must be unique.
	// Keep records of ones we've already seen.
	names := stringset.New(len(cfg.OperatingSystem))
	for _, os := range cfg.OperatingSystem {
		switch {
		case os.Name == "":
			c.Errorf("operating system names are required and must be non-empty")
		case !names.Add(os.Name):
			c.Errorf("duplicate operating system %q", os.Name)
		}
	}
}
