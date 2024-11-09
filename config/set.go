// Copyright 2018 The LUCI Authors.
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
	"fmt"
	"regexp"
	"strings"
)

// ServiceNamePattern is the regexp pattern string that matches valid service
// name.
const ServiceNamePattern = `[a-z0-9\-_]+`

// serviceNameRegexp matches valid service name
var serviceNameRegexp = regexp.MustCompile(fmt.Sprintf(`^%s$`, ServiceNamePattern))

// Set is a name of a configuration set: a bunch of config files versioned and
// stored as a single unit in a same repository.
//
// A config set name consists of a domain and a target.
//
//   - Service config sets are config sets in the "services" domain, with the
//     service name as the target.
//   - Project config sets are config sets in the "projects" domain. The target
//     is the project name.
type Set string

// Domain describes the domain of the config set.
type Domain string

const (
	// ProjectDomain is the domain of project config set(e.g. projects/chromium).
	ProjectDomain Domain = "projects"
	// ServiceDomain is the domain of service config set (e.g.
	// services/luci-config).
	ServiceDomain Domain = "services"
)

// ServiceSet returns the name of a config set for the specified service.
//
// Returns error if the service name doesn't match `ServiceNamePattern`.
func ServiceSet(service string) (Set, error) {
	if !serviceNameRegexp.MatchString(service) {
		return "", fmt.Errorf("invalid service name %q, expected to match %q", service, ServiceNamePattern)
	}
	return Set(fmt.Sprintf("%s/%s", ServiceDomain, service)), nil
}

// MustServiceSet is like `ServiceSet` but panic on invalid service name.
func MustServiceSet(service string) Set {
	cs, err := ServiceSet(service)
	if err != nil {
		panic(err)
	}
	return cs
}

// ProjectSet returns the config set for the specified project.
//
// Returns error if the project name is invalid. See `ValidateProjectName`.
func ProjectSet(project string) (Set, error) {
	if err := ValidateProjectName(project); err != nil {
		return "", fmt.Errorf("invalid project name: %w", err)
	}
	return Set(fmt.Sprintf("%s/%s", ProjectDomain, project)), nil
}

// MustProjectSet is like `ProjectSet` but panic on invalid project name.
func MustProjectSet(project string) Set {
	cs, err := ProjectSet(project)
	if err != nil {
		panic(err)
	}
	return cs
}

// Split splits a Set into its domain, target components.
func (cs Set) Split() (domain Domain, target string) {
	p := strings.SplitN(string(cs), "/", 2)
	if len(p) == 1 {
		return Domain(p[0]), ""
	}
	return Domain(p[0]), p[1]
}

// Service returns a service name for a service config set or empty string for
// all other sets.
func (cs Set) Service() string {
	domain, target := cs.Split()
	if domain == ServiceDomain {
		return target
	}
	return ""
}

// Project returns a project name for a project config set or empty string for
// all other sets.
func (cs Set) Project() string {
	domain, target := cs.Split()
	if domain == ProjectDomain {
		return target
	}
	return ""
}

// Domain returns the domain of the config set.
func (cs Set) Domain() Domain {
	domain, _ := cs.Split()
	return domain
}

// Validate checks that the config set is well-formed.
func (cs Set) Validate() error {
	switch domain, target := cs.Split(); domain {
	case "":
		return fmt.Errorf("can not extract domain from config set %q. expected syntax \"domain/target\"", cs)
	case ProjectDomain:
		_, err := ProjectSet(target)
		return err
	case ServiceDomain:
		_, err := ServiceSet(target)
		return err
	default:
		return fmt.Errorf("unknown domain %q for config set %q; currently supported domains [%s, %s]", domain, cs, ProjectDomain, ServiceDomain)
	}
}
