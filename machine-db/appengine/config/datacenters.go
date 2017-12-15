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
	"go.chromium.org/luci/machine-db/appengine/model"
)

// datacentersConfig is the name of the config file enumerating datacenter files.
const datacentersConfig = "datacenters.cfg"

// switchMaxPorts is the maximum number of ports a switch may have.
const switchMaxPorts = 65535

// importDatacenterConfigs fetches, validates, and applies datacenter configs.
func importDatacenterConfigs(c context.Context, configSet cfgtypes.ConfigSet) error {
	datacenterFiles := &config.DatacentersConfig{}
	metadata := &cfgclient.Meta{}
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, datacentersConfig, textproto.Message(datacenterFiles), metadata); err != nil {
		return errors.Annotate(err, "failed to load config: %s", datacentersConfig).Err()
	}
	logging.Infof(c, "Found config revision: %s", metadata.Revision)

	validationContext := &validation.Context{Context: c}
	validateDatacentersConfig(validationContext, datacenterFiles)
	if err := validationContext.Finalize(); err != nil {
		return errors.Annotate(err, "failed to validate config").Err()
	}

	// datacenterConfigFiles will be a map of datacenter config filename to DatacenterConfig.
	datacenterConfigFiles := make(map[string]*config.DatacenterConfig, len(datacenterFiles.Datacenter))
	// datacenterConfigs will be a slice of DatacenterConfigs.
	datacenterConfigs := make([]*config.DatacenterConfig, 0, len(datacenterConfigFiles))
	for _, datacenterFile := range datacenterFiles.Datacenter {
		datacenter := &config.DatacenterConfig{}
		if err := cfgclient.Get(c, cfgclient.AsService, configSet, datacenterFile, textproto.Message(datacenter), nil); err != nil {
			return errors.Annotate(err, "failed to load datacenter config %q", datacenterFile).Err()
		}
		datacenterConfigFiles[datacenterFile] = datacenter
		datacenterConfigs = append(datacenterConfigs, datacenter)
		logging.Infof(c, "Found configured datacenter %q", datacenter.Name)
	}

	validateDatacenters(validationContext, datacenterConfigFiles)
	if err := validationContext.Finalize(); err != nil {
		return errors.Annotate(err, "failed to validate config").Err()
	}

	datacenterIds, err := model.EnsureDatacenters(c, datacenterConfigs)
	if err != nil {
		return errors.Annotate(err, "failed to ensure datacenters").Err()
	}
	rackIds, err := model.EnsureRacks(c, datacenterConfigs, datacenterIds)
	if err != nil {
		return errors.Annotate(err, "failed to ensure racks").Err()
	}
	err = model.EnsureSwitches(c, datacenterConfigs, rackIds)
	if err != nil {
		return errors.Annotate(err, "failed to ensure switches").Err()
	}

	return nil
}

// validateDatacentersConfig validates datacenters.cfg.
func validateDatacentersConfig(c *validation.Context, datacentersConfig *config.DatacentersConfig) error {
	// Datacenter filenames must be unique.
	// Keep records of which ones we've already seen.
	files := stringset.New(len(datacentersConfig.Datacenter))

	c.SetFile("datacenters.cfg")
	for _, file := range datacentersConfig.Datacenter {
		if file == "" {
			c.Errorf("datacenter filenames are required and must be non-empty")
		}
		if files.Has(file) {
			c.Errorf("duplicate filename %q", file)
		}
		files.Add(file)
	}
	return nil
}

// validateDatacenters validates the individual datacenter.cfg files referenced in datacenters.cfg.
func validateDatacenters(c *validation.Context, datacenterConfigs map[string]*config.DatacenterConfig) error {
	// Datacenter, rack, and switch names must be unique.
	// Keep records of ones we've already seen.
	datacenters := stringset.New(len(datacenterConfigs))
	racks := stringset.New(0)
	switches := stringset.New(0)

	for datacenter, datacenterConfig := range datacenterConfigs {
		c.SetFile(datacenter)
		if datacenterConfig.Name == "" {
			c.Errorf("datacenter names are required and must be non-empty")
		}
		if datacenters.Has(datacenterConfig.Name) {
			c.Errorf("duplicate datacenter %q", datacenterConfig.Name)
		}
		datacenters.Add(datacenterConfig.Name)

		c.Enter("datacenter: %q", datacenterConfig.Name)
		for _, rackConfig := range datacenterConfig.Rack {
			if rackConfig.Name == "" {
				c.Errorf("rack names are required and must be non-empty")
			}
			if racks.Has(rackConfig.Name) {
				c.Errorf("duplicate rack %q", rackConfig.Name)
			}
			racks.Add(rackConfig.Name)

			c.Enter("rack: %q", rackConfig.Name)
			for _, switchConfig := range rackConfig.Switch {
				if switchConfig.Name == "" {
					c.Errorf("switch names are required and must be non-empty")
				}
				if switches.Has(switchConfig.Name) {
					c.Errorf("duplicate switch %q", switchConfig.Name)
				}
				switches.Add(switchConfig.Name)

				c.Enter("switch: %q", switchConfig.Name)
				if switchConfig.Ports < 1 {
					c.Errorf("switch %q must have at least one port", switchConfig.Name)
				}
				if switchConfig.Ports > switchMaxPorts {
					c.Errorf("switch %q must have at most %d ports", switchConfig.Name, switchMaxPorts)
				}
				c.Exit()
			}
			c.Exit()
		}
		c.Exit()
	}
	return nil
}
