// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"strings"

	"github.com/luci/luci-go/common/config"
)

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
