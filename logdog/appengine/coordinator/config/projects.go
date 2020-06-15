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

	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
)

// ProjectConfigPath returns the path of the project-specific configuration.
// This path should be used with a project config set.
//
// A given project's configuration is named after the current App ID.
func ProjectConfigPath(c context.Context) string {
	return cfgclient.CurrentServiceName(c) + ".cfg"
}

// ProjectConfig loads the project config protobuf from the config service.
//
// This function will return:
//	- nil, if the project exists and the configuration successfully loaded
//	- config.ErrNoConfig if the project configuration was not present.
//	- ErrInvalidConfig if the project configuration was present, but could not
//	  be loaded.
//	- Some other error if an error occurred that does not fit one of the
//	  previous categories.
func ProjectConfig(c context.Context, project string) (*svcconfig.ProjectConfig, error) {
	if project == "" {
		return nil, config.ErrNoConfig
	}

	// Get the config from the config service. If the configuration doesn't exist,
	// this will return config.ErrNoConfig.
	configSet, configPath := config.ProjectSet(project), ProjectConfigPath(c)

	var pcfg svcconfig.ProjectConfig
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, configPath, textproto.Message(&pcfg), nil); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    project,
			"configSet":  configSet,
			"configPath": configPath,
		}.Errorf(c, "Failed to load project configuration content.")
		return nil, err
	}
	return &pcfg, nil
}

// ActiveProjects returns a full list of all config service projects with
// LogDog project configurations.
//
// The list will be alphabetically sorted.
func ActiveProjects(c context.Context) ([]string, error) {
	configPath := ProjectConfigPath(c)

	var metas []*config.Meta
	if err := cfgclient.Projects(c, cfgclient.AsService, configPath, nil, &metas); err != nil {
		log.WithError(err).Errorf(c, "Failed to load project configs.")
		return nil, err
	}

	// Iterate through our Metas and extract the project names.
	projects := make([]string, 0, len(metas))
	for _, meta := range metas {
		if projectName := meta.ConfigSet.Project(); projectName != "" {
			projects = append(projects, projectName)
		}
	}
	sort.Strings(projects)
	return projects, nil
}
