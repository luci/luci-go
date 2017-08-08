// Copyright 2016 The LUCI Authors.
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

package main

import (
	"path/filepath"

	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/deploytool/api/deploy"

	"github.com/mitchellh/go-homedir"
	"golang.org/x/net/context"
)

const userCfgName = ".luci_deploy.cfg"

func loadUserConfig(c context.Context, cfg *deploy.UserConfig) error {
	path, err := homedir.Dir()
	if err != nil {
		log.WithError(err).Warningf(c, "Failed to get user home directory; cannot load local config.")
		return nil
	}

	configPath := filepath.Join(path, userCfgName)
	switch err := unmarshalTextProtobuf(configPath, cfg); {
	case err == nil:
		log.Infof(c, "Loaded user config from: %s", configPath)

	case isNotExist(err):
		log.Debugf(c, "No user config found at: %s", configPath)

	default:
		return errors.Annotate(err, "failed to load config at [%s]", configPath).Err()
	}

	return nil
}
