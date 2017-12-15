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

	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/server/router"
)

// configImportHandler handles HTTP requests to reimport the config.
func configImportHandler(c *router.Context) {
	c.Writer.Header().Set("Content-Type", "text/plain")

	if err := importConfigs(c.Context); err != nil {
		errors.Log(c.Context, err)
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	c.Writer.WriteHeader(http.StatusOK)
}

// importConfigs fetches, validates, and applies configs from the config service.
func importConfigs(c context.Context) error {
	configSet := cfgclient.CurrentServiceConfigSet(c)
	if err := importDatacenterConfigs(c, configSet); err != nil {
		return errors.Annotate(err, "failed to import datacenters").Err()
	}
	if err := importOSConfigs(c, configSet); err != nil {
		return errors.Annotate(err, "failed to import operating systems").Err()
	}
	// TODO(smut): Import the rest of the configs.
	return nil
}

// InstallHandlers installs handlers for HTTP requests pertaining to configs.
func InstallHandlers(r *router.Router, middleware router.MiddlewareChain) {
	cronMiddleware := middleware.Extend(gaemiddleware.RequireCron)
	r.GET("/internal/cron/import-config", cronMiddleware, configImportHandler)
}
