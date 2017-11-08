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

package database

import (
	"fmt"
	"net/http"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/textproto"
	"go.chromium.org/luci/machine-db/api/config/v1"
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

	configSet := cfgclient.CurrentServiceConfigSet(c.Context)
	datacenterFiles := &config.DatacentersConfig{}
	if err := cfgclient.Get(c.Context, cfgclient.AsService, configSet, datacentersConfig, textproto.Message(datacenterFiles), nil); err != nil {
		logging.Errorf(c.Context, fmt.Sprintf("Failed to load %s: %s", datacentersConfig, err.Error()))
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	var datacenterConfigs []*config.DatacenterConfig
	for _, datacenterFile := range datacenterFiles.Datacenter {
		datacenter := &config.DatacenterConfig{}
		datacenterFile = fmt.Sprintf("datacenters/%s", datacenterFile)
		if err := cfgclient.Get(c.Context, cfgclient.AsService, configSet, datacenterFile, textproto.Message(datacenter), nil); err != nil {
			logging.Errorf(c.Context, fmt.Sprintf("Failed to load %s: %s", datacenterFile, err.Error()))
			c.Writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		datacenterConfigs = append(datacenterConfigs, datacenter)
		logging.Infof(c.Context, "Found configured datacenter: %s", datacenter.Name)
	}

	if err := validateDatacenters(c.Context, datacenterConfigs); err != nil {
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := EnsureDatacenters(c.Context, datacenterConfigs); err != nil {
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	c.Writer.WriteHeader(http.StatusOK)
}

// InstallConfigHandlers installs handlers for HTTP requests pertaining to configs.
func InstallConfigHandlers(r *router.Router) {
	cronMiddleware := standard.Base().Extend(gaemiddleware.RequireCron)
	r.GET("/internal/cron/import-config", cronMiddleware, configImportHandler)
}

// validateDatacenters validates datacenters.cfg.
func validateDatacenters(c context.Context, datacenterConfigs []*config.DatacenterConfig) error {
	// Datacenter, rack, and switch names must be unique.
	// Keep maps of ones we've already seen.
	datacenters := make(map[string]struct{})
	racks := make(map[string]struct{})
	switches := make(map[string]struct{})

	for _, datacenterConfig := range datacenterConfigs {
		if datacenterConfig.Name == "" {
			return fmt.Errorf("Datacenter names are required and must be non-empty")
		}
		if _, ok := datacenters[datacenterConfig.Name]; ok {
			return fmt.Errorf("Duplicate datacenter: %s", datacenterConfig.Name)
		}
		datacenters[datacenterConfig.Name] = struct{}{}

		for _, rackConfig := range datacenterConfig.Rack {
			if rackConfig.Name == "" {
				return fmt.Errorf("Rack names are required and must be non-empty")
			}
			if _, ok := racks[rackConfig.Name]; ok {
				return fmt.Errorf("Duplicate rack: %s", rackConfig.Name)
			}
			racks[rackConfig.Name] = struct{}{}

			for _, switchConfig := range rackConfig.Switch {
				if switchConfig.Name == "" {
					return fmt.Errorf("Switch names are required and must be non-empty")
				}
				if _, ok := switches[switchConfig.Name]; ok {
					return fmt.Errorf("Duplicate switch: %s", switchConfig.Name)
				}
				switches[switchConfig.Name] = struct{}{}
				if switchConfig.Ports < 1 {
					return fmt.Errorf("Switch must have at least one port: %s", switchConfig.Name)
				}
				if switchConfig.Ports > switchMaxPorts {
					return fmt.Errorf("Switch must have at most %d ports: %s", switchMaxPorts, switchConfig.Name)
				}
			}
		}
	}

	return nil
}
