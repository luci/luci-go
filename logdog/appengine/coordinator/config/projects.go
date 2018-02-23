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
	"sort"

	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/logdog/api/config/svcconfig"

	"golang.org/x/net/context"
)

// ProjectConfigPath returns the path of the project-specific configuration.
// This path should be used with a project config set.
//
// A given project's configuration is named after the current App ID.
func ProjectConfigPath(c context.Context) string {
	return svcconfig.ProjectConfigPath(cfgclient.CurrentServiceName(c))
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
func ProjectConfig(c context.Context, project config.ProjectName) (*svcconfig.ProjectConfig, error) {
	if project == "" {
		return nil, cfgclient.ErrNoConfig
	}

	// Get the config from the config service. If the configuration doesn't exist,
	// this will return cfgclient.ErrNoConfig.
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

// ProjectNames returns a sorted list of the names of all of the projects
// that the supplied authority can view.
func ProjectNames(c context.Context, a cfgclient.Authority) ([]config.ProjectName, error) {
	configPath := ProjectConfigPath(c)

	var metas []*cfgclient.Meta
	if err := cfgclient.Projects(c, a, configPath, nil, &metas); err != nil {
		log.WithError(err).Errorf(c, "Failed to load project configs.")
		return nil, err
	}

	// Iterate through our Metas and extract the project names.
	projects := make([]config.ProjectName, 0, len(metas))
	for _, meta := range metas {
		projectName, _, _ := meta.ConfigSet.SplitProject()
		if projectName != "" {
			projects = append(projects, projectName)
		}
	}
	sort.Sort(projectNameSlice(projects))
	return projects, nil
}

// ActiveProjects returns a full list of all config service projects with
// LogDog project configurations.
//
// The list will be alphabetically sorted.
func ActiveProjects(c context.Context) ([]config.ProjectName, error) {
	return ProjectNames(c, cfgclient.AsService)
}

// ActiveUserProjects returns a full list of all config service projects with
// LogDog project configurations that the current user can see.
//
// The list will be alphabetically sorted.
func ActiveUserProjects(c context.Context) ([]config.ProjectName, error) {
	return ProjectNames(c, cfgclient.AsUser)
}

type projectNameSlice []config.ProjectName

func (s projectNameSlice) Len() int           { return len(s) }
func (s projectNameSlice) Less(i, j int) bool { return s[i] < s[j] }
func (s projectNameSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
