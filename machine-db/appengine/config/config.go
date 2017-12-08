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
	"net/http"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/textproto"
	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/model/datacenters"
	"go.chromium.org/luci/server/router"

	"golang.org/x/net/context"
)

// datacentersConfig is the name of the config file enumerating datacenter files.
const datacentersConfig = "datacenters.cfg"

// switchMaxPorts is the maximum number of ports a switch may have.
const switchMaxPorts = 65535

// configImportHandler handles HTTP requests to reimport the config.
func configImportHandler(c *router.Context) {
	c.Writer.Header().Set("Content-Type", "text/plain")

	if err := importConfigs(c.Context); err != nil {
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	c.Writer.WriteHeader(http.StatusOK)
}

// importConfigs fetches and validates configs from the config service.
func importConfigs(c context.Context) error {
	configSet := cfgclient.CurrentServiceConfigSet(c)
	datacenterFiles := &config.DatacentersConfig{}
	metadata := &cfgclient.Meta{}
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, datacentersConfig, textproto.Message(datacenterFiles), metadata); err != nil {
		logging.Errorf(c, "Failed to load %s: %s", datacentersConfig, err.Error())
		return err
	}
	logging.Infof(c, "Found config revision: %s", metadata.Revision)

	validationContext := &validation.Context{Logger: logging.Get(c)}
	validateDatacentersConfig(validationContext, datacenterFiles)
	if err := validationContext.Finalize(); err != nil {
		logging.Errorf(c, "Failed to validate the config: %s", err.Error())
		return err
	}

	// datacenterConfigFiles will be a map of datacenter config filename to DatacenterConfig.
	datacenterConfigFiles := make(map[string]*config.DatacenterConfig, len(datacenterFiles.Datacenter))
	// datacenterConfigs will be an array of DatacenterConfigs.
	datacenterConfigs := make([]*config.DatacenterConfig, 0, len(datacenterConfigFiles))
	for _, datacenterFile := range datacenterFiles.Datacenter {
		datacenter := &config.DatacenterConfig{}
		if err := cfgclient.Get(c, cfgclient.AsService, configSet, datacenterFile, textproto.Message(datacenter), nil); err != nil {
			logging.Errorf(c, "Failed to load %s: %s", datacenterFile, err.Error())
			return err
		}
		datacenterConfigFiles[datacenterFile] = datacenter
		datacenterConfigs = append(datacenterConfigs, datacenter)
		logging.Infof(c, "Found configured datacenter: %s", datacenter.Name)
	}

	validateDatacenters(validationContext, datacenterConfigFiles)
	if err := validationContext.Finalize(); err != nil {
		logging.Errorf(c, "Failed to validate the config: %s", err.Error())
		return err
	}

	if err := datacenters.EnsureDatacenters(c, datacenterConfigs); err != nil {
		logging.Errorf(c, "Failed to ensure datacenters: %s", err.Error())
		return err
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
			c.Errorf("duplicate filename: %s", file)
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
			c.Errorf("duplicate datacenter: %s", datacenterConfig.Name)
		}
		datacenters.Add(datacenterConfig.Name)

		c.Enter("datacenter: %q", datacenterConfig.Name)
		for _, rackConfig := range datacenterConfig.Rack {
			if rackConfig.Name == "" {
				c.Errorf("rack names are required and must be non-empty")
			}
			if racks.Has(rackConfig.Name) {
				c.Errorf("duplicate rack: %s", rackConfig.Name)
			}
			racks.Add(rackConfig.Name)

			c.Enter("rack: %q", rackConfig.Name)
			for _, switchConfig := range rackConfig.Switch {
				if switchConfig.Name == "" {
					c.Errorf("switch names are required and must be non-empty")
				}
				if switches.Has(switchConfig.Name) {
					c.Errorf("duplicate switch: %s", switchConfig.Name)
				}
				switches.Add(switchConfig.Name)

				c.Enter("switch: %q", switchConfig.Name)
				if switchConfig.Ports < 1 {
					c.Errorf("switch must have at least one port: %s", switchConfig.Name)
				}
				if switchConfig.Ports > switchMaxPorts {
					c.Errorf("switch must have at most %d ports: %s", switchMaxPorts, switchConfig.Name)
				}
				c.Exit()
			}
			c.Exit()
		}
		c.Exit()
	}
	return nil
}

// InstallHandlers installs handlers for HTTP requests pertaining to configs.
func InstallHandlers(r *router.Router, middleware router.MiddlewareChain) {
	cronMiddleware := middleware.Extend(gaemiddleware.RequireCron)
	r.GET("/internal/cron/import-config", cronMiddleware, configImportHandler)
}
