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

package coordinator

import (
	"context"
	"strings"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/common/types"
)

const (
	// ProjectNamespacePrefix is the datastore namespace prefix for project
	// namespaces.
	ProjectNamespacePrefix = "luci."
)

// ProjectNamespace returns the AppEngine namespace for a given luci-config
// project name.
func ProjectNamespace(project types.ProjectName) string {
	return ProjectNamespacePrefix + string(project)
}

// ProjectFromNamespace returns the current project installed in the supplied
// Context's namespace.
//
// If the namespace does not have a project namespace prefix, this function
// will return an empty string.
func ProjectFromNamespace(ns string) types.ProjectName {
	if !strings.HasPrefix(ns, ProjectNamespacePrefix) {
		return ""
	}
	return types.ProjectName(ns[len(ProjectNamespacePrefix):])
}

// CurrentProject returns the current project based on the currently-loaded
// namespace.
//
// If there is no current namespace, or if the current namespace is not a valid
// project namespace, an empty string will be returned.
func CurrentProject(c context.Context) types.ProjectName {
	if ns := info.GetNamespace(c); ns != "" {
		return ProjectFromNamespace(ns)
	}
	return ""
}

// CurrentProjectConfig returns the project-specific configuration for the
// current project.
//
// If there is no current project namespace, or if the current project has no
// configuration, config.ErrInvalidConfig will be returned.
func CurrentProjectConfig(c context.Context) (*svcconfig.ProjectConfig, error) {
	return GetConfigProvider(c).ProjectConfig(c, CurrentProject(c))
}
