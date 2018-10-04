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
	"context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/config/validation"

	configPB "go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/model"
)

// osesFilename is the name of the config file enumerating operating systems.
const osesFilename = "oses.cfg"

// importOSes fetches, validates, and applies operating system configs.
func importOSes(c context.Context, configSet config.Set) error {
	os := &configPB.OSes{}
	metadata := &config.Meta{}
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, osesFilename, textproto.Message(os), metadata); err != nil {
		return errors.Annotate(err, "failed to load %s", osesFilename).Err()
	}
	logging.Infof(c, "Found %s revision %q", osesFilename, metadata.Revision)

	ctx := &validation.Context{Context: c}
	ctx.SetFile(osesFilename)
	validateOSes(ctx, os)
	if err := ctx.Finalize(); err != nil {
		return errors.Annotate(err, "invalid config").Err()
	}

	if err := model.EnsureOSes(c, os.OperatingSystem); err != nil {
		return errors.Annotate(err, "failed to ensure operating systems").Err()
	}
	return nil
}

// validateOSes validates oses.cfg.
func validateOSes(c *validation.Context, cfg *configPB.OSes) {
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
