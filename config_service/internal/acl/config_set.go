// Copyright 2023 The LUCI Authors.
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

package acl

import (
	"context"
	"fmt"

	"go.chromium.org/luci/config"
)

// CanReadConfigSets checks whether the requester can read the config sets.
//
// Returns a bitmap that maps to the provided config sets.
func CanReadConfigSets(ctx context.Context, sets []config.Set) ([]bool, error) {
	projects := make([]string, 0, len(sets))
	projectIndices := make([]int, 0, len(sets))
	services := make([]string, 0, len(sets))
	servicesIndices := make([]int, 0, len(sets))

	for i, cs := range sets {
		switch domain, target := cs.Split(); domain {
		case config.ProjectDomain:
			projects = append(projects, target)
			projectIndices = append(projectIndices, i)
		case config.ServiceDomain:
			services = append(services, target)
			servicesIndices = append(servicesIndices, i)
		default:
			return nil, fmt.Errorf("unknown domain: %s", cs)
		}
	}

	ret := make([]bool, len(sets))
	if len(projects) > 0 {
		projResults, err := CanReadProjects(ctx, projects)
		if err != nil {
			return nil, err
		}
		for i, res := range projResults {
			ret[projectIndices[i]] = res
		}
	}

	if len(services) > 0 {
		srvResults, err := CanReadServices(ctx, services)
		if err != nil {
			return nil, err
		}
		for i, res := range srvResults {
			ret[servicesIndices[i]] = res
		}
	}
	return ret, nil
}

// CanReadConfigSet checks whether the requester can read the provided config
// set.
func CanReadConfigSet(ctx context.Context, cs config.Set) (bool, error) {
	switch domain, target := cs.Split(); domain {
	case config.ProjectDomain:
		return CanReadProject(ctx, target)
	case config.ServiceDomain:
		return CanReadService(ctx, target)
	default:
		return false, fmt.Errorf("unknown domain: %s", domain)
	}
}

// CanValidateConfigSet checks whether the requester can validate the provided
// config set.
func CanValidateConfigSet(ctx context.Context, cs config.Set) (bool, error) {
	switch domain, target := cs.Split(); domain {
	case config.ProjectDomain:
		return CanValidateProject(ctx, target)
	case config.ServiceDomain:
		return CanValidateService(ctx, target)
	default:
		return false, fmt.Errorf("unknown domain: %s", cs)
	}
}

// CanReimportConfigSet checks whether the requester can reimport the provided
// config set.
func CanReimportConfigSet(ctx context.Context, cs config.Set) (bool, error) {
	switch domain, target := cs.Split(); domain {
	case config.ProjectDomain:
		return CanReimportProject(ctx, target)
	case config.ServiceDomain:
		return CanReimportService(ctx, target)
	default:
		return false, fmt.Errorf("unknown domain: %s", cs)
	}
}
