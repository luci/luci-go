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
	"sort"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
)

// ProjectConfig loads the project config protobuf from the config service.
//
// This function will return following errors:
//	- nil, if the project exists and the configuration successfully loaded
//	- config.ErrNoConfig if the project configuration was not present.
//	- ErrInvalidConfig if the project configuration was present, but could not
//	  be loaded.
//	- Some other error if an error occurred that does not fit one of the
//	  previous categories.
func ProjectConfig(ctx context.Context, project string) (*svcconfig.ProjectConfig, error) {
	store := store(ctx)

	// TODO(vadimsh): Really add caching.

	if project == "" {
		return nil, config.ErrNoConfig
	}

	var pcfg svcconfig.ProjectConfig
	err := cfgclient.Get(
		ctx,
		cfgclient.AsService,
		config.ProjectSet(project),
		store.serviceID+".cfg",
		textproto.Message(&pcfg),
		nil,
	)
	if err != nil {
		logging.Errorf(ctx, "Failed to load config for project %q: %s", project, err)
		return nil, err
	}

	return &pcfg, nil
}

// ActiveProjects returns a list of all projects with LogDog project configs.
//
// The list will be alphabetically sorted.
func ActiveProjects(ctx context.Context) ([]string, error) {
	store := store(ctx)

	// TODO(vadimsh): Really add caching.

	var metas []*config.Meta
	if err := cfgclient.Projects(ctx, cfgclient.AsService, store.serviceID+".cfg", nil, &metas); err != nil {
		logging.Errorf(ctx, "Failed to load project configs: %s", err)
		return nil, err
	}

	projects := make([]string, 0, len(metas))
	for _, meta := range metas {
		if projectName := meta.ConfigSet.Project(); projectName != "" {
			projects = append(projects, projectName)
		}
	}
	sort.Strings(projects)
	return projects, nil
}
