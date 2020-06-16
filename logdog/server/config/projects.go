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
	"context"
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
)

// missingProjectMarker is cached instead of *svcconfig.ProjectConfig if the
// project is missing to avoid hitting cfgclient all the time when accessing
// missing projects.
//
// Note: strictly speaking caching all missing projects forever in
// Store.projects introduces a DoS attack vector. But this code is scheduled for
// removal when Logdog is integrated with LUCI Realms, so it's fine to ignore
// this problem for now.
var missingProjectMarker = "missing project"

// ProjectConfig loads the project config protobuf from the config service.
//
// This function will return following errors:
//	- nil, if the project exists and the configuration successfully loaded
//	- config.ErrNoConfig if the project configuration was not present.
//	- ErrInvalidConfig if the project configuration was present, but could not
//	  be loaded.
//	- Some other error if an error occurred that does not fit one of the
//	  previous categories.
func ProjectConfig(ctx context.Context, projectID string) (*svcconfig.ProjectConfig, error) {
	store := store(ctx)
	if projectID == "" {
		return nil, config.ErrNoConfig
	}
	if store.NoCache {
		return fetchProjectConfig(ctx, projectID, store.serviceID)
	}
	cached, err := store.projectCacheSlot(projectID).Get(ctx, func(prev interface{}) (val interface{}, exp time.Duration, err error) {
		logging.Infof(ctx, "Cache miss for %q project config, fetching it...", projectID)
		cfg, err := fetchProjectConfig(ctx, projectID, store.serviceID)
		if err == config.ErrNoConfig {
			return &missingProjectMarker, time.Minute, nil
		}
		return cfg, 5 * time.Minute, err
	})
	if err != nil {
		return nil, err
	}
	if cached == &missingProjectMarker {
		return nil, config.ErrNoConfig
	}
	return cached.(*svcconfig.ProjectConfig), nil
}

// fetchProjectConfig fetches a project config from the storage.
func fetchProjectConfig(ctx context.Context, projectID, serviceID string) (*svcconfig.ProjectConfig, error) {
	var pcfg svcconfig.ProjectConfig
	err := cfgclient.Get(
		ctx,
		cfgclient.AsService,
		config.ProjectSet(projectID),
		serviceID+".cfg",
		textproto.Message(&pcfg),
		nil,
	)
	if err != nil {
		logging.Errorf(ctx, "Failed to load config for project %q: %s", projectID, err)
		return nil, err
	}
	return &pcfg, nil
}
