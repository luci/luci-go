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

package cfgtypes

import (
	"strings"
)

// ConfigSet is a configuration service Config Set string.
//
// A config set consists of a domain and a series of path components:
// domain/target/refs...
//
//	- Service config sets are config sets in the "services" domain, with the
//	  service name as the target.
//	- Project config sets are config sets in the "projects" domain. The target
//	  is the project name.
type ConfigSet string

// ServiceConfigSet returns the name of a config set for the specified service.
func ServiceConfigSet(name string) ConfigSet { return ConfigSet("services/" + name) }

// ProjectConfigSet returns the config set for the specified project.
func ProjectConfigSet(name ProjectName) ConfigSet { return RefConfigSet(name, "") }

// RefConfigSet returns the config set for the specified project and ref. If ref
// is empty, this will equal the ProjectConfigSet value.
func RefConfigSet(name ProjectName, ref string) ConfigSet {
	if ref == "" {
		return ConfigSet("projects/" + string(name))
	}
	return ConfigSet(strings.Join([]string{"projects", string(name), ref}, "/"))
}

// Split splits a ConfigSet into its domain, target, and ref components.
func (cs ConfigSet) Split() (domain, target, ref string) {
	switch p := strings.SplitN(string(cs), "/", 3); len(p) {
	case 1:
		return p[0], "", ""
	case 2:
		return p[0], p[1], ""
	default:
		return p[0], p[1], p[2]
	}
}

// SplitProject splits a project-rooted config set (projects/<name>[/...]) into
// its project name, project config set, and ref components.
//
// For example, "projects/foo/bar/baz" is a config set that belongs to project
// "foo". It will be parsed into:
//	- project: "foo"
//	- projectConfigSet: "projects/foo"
//	- ref: "bar/baz"
//
// If configSet is not a project config set, empty strings will be returned.
func (cs ConfigSet) SplitProject() (project ProjectName, projectConfigSet ConfigSet, ref string) {
	parts := strings.SplitN(string(cs), "/", 3)
	if len(parts) < 2 || parts[0] != "projects" {
		// Not a project config set, so neither remaining Authority can access.
		return
	}

	// The project is the first part.
	project = ProjectName(parts[1])

	// Re-assemble the project part of the config set ("projects/foo").
	projectConfigSet = cs[:len("projects/")+len(project)]
	if len(parts) > 2 {
		ref = parts[2]
	}
	return
}
