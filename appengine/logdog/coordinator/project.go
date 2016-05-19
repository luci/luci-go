// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"strings"

	"github.com/luci/gae/service/datastore/meta"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"golang.org/x/net/context"
)

// projectNamespacePrefix is the datastore namespace prefix for project
// namespaces.
const projectNamespacePrefix = "luci."

// ProjectNamespace returns the AppEngine namespace for a given luci-config
// project name.
func ProjectNamespace(project config.ProjectName) string {
	return projectNamespacePrefix + string(project)
}

// ProjectFromNamespace returns the current project installed in the supplied
// Context's namespace.
//
// If the namespace does not have a project namespace prefix, this function
// will return an empty string.
func ProjectFromNamespace(ns string) config.ProjectName {
	if !strings.HasPrefix(ns, projectNamespacePrefix) {
		return ""
	}
	return config.ProjectName(ns[len(projectNamespacePrefix):])
}

// CurrentProject returns the current project based on the currently-loaded
// namespace.
//
// If there is no current namespace, or if the current namespace is not a valid
// project namespace, an empty string will be returned.
func CurrentProject(c context.Context) config.ProjectName {
	if ns, ok := info.Get(c).GetNamespace(); ok {
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
	return GetServices(c).ProjectConfig(c, CurrentProject(c))
}

// AllProjectsWithNamespaces scans current namespaces and returns those that
// belong to LUCI projects.
func AllProjectsWithNamespaces(c context.Context) ([]config.ProjectName, error) {
	var projects []config.ProjectName
	err := meta.NamespacesWithPrefix(c, projectNamespacePrefix, func(ns string) error {
		if proj := ProjectFromNamespace(ns); proj != "" {
			projects = append(projects, proj)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return projects, nil
}
