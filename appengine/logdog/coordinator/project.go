// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"strings"

	"github.com/luci/gae/service/datastore/meta"
	"github.com/luci/luci-go/common/config"
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
