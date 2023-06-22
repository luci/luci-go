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
	"errors"
	"fmt"

	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
)

// CanReadProjects checks whether the requester can read the provided projects.
//
// Returns a bitmap that maps to the provided projects.
func CanReadProjects(ctx context.Context, projects []string) ([]bool, error) {
	if len(projects) == 0 {
		return nil, errors.New("expected non-empty projects list, got empty")
	}
	ret := make([]bool, len(projects))
	aclCfg, err := getACLCfgCached(ctx)
	if err != nil {
		return nil, err
	}
	for i, proj := range projects {
		ret[i], err = checkProjectPerm(ctx, proj, ReadPermission, aclCfg)
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}

// CanReadProject checks whether the requester can read the config for the
// provided project
func CanReadProject(ctx context.Context, project string) (bool, error) {
	aclCfg, err := getACLCfgCached(ctx)
	if err != nil {
		return false, err
	}
	return checkProjectPerm(ctx, project, ReadPermission, aclCfg)
}

// CanValidateProject checks whether the requester can validate the config
// for the provided project.
func CanValidateProject(ctx context.Context, project string) (bool, error) {
	aclCfg, err := getACLCfgCached(ctx)
	if err != nil {
		return false, err
	}
	// No actions are allowed if no read access to the Project
	switch allowed, err := checkProjectPerm(ctx, project, ReadPermission, aclCfg); {
	case err != nil:
		return false, err
	case !allowed:
		return false, nil
	}

	return checkProjectPerm(ctx, project, ValidatePermission, aclCfg)
}

// CanReimportProject checks whether the requester can reimport the config for
// the provided project.
func CanReimportProject(ctx context.Context, project string) (bool, error) {
	aclCfg, err := getACLCfgCached(ctx)
	if err != nil {
		return false, err
	}
	// No actions are allowed if no read access to the Project
	switch allowed, err := checkProjectPerm(ctx, project, ReadPermission, aclCfg); {
	case err != nil:
		return false, err
	case !allowed:
		return false, nil
	}

	return checkProjectPerm(ctx, project, ReimportPermission, aclCfg)
}

func checkProjectPerm(ctx context.Context, project string, perm realms.Permission, aclCfg *cfgcommonpb.AclCfg) (bool, error) {
	if project == "" {
		return false, errors.New("expected non-empty project, got empty")
	}
	var equivalentGroup string
	switch perm {
	case ReadPermission:
		equivalentGroup = aclCfg.GetProjectAccessGroup()
	case ValidatePermission:
		equivalentGroup = aclCfg.GetProjectValidationGroup()
	case ReimportPermission:
		equivalentGroup = aclCfg.GetProjectReimportGroup()
	}
	// Checking If a caller is allowed in the global level groups first.
	// If the caller is allowed, no need to check per-project level perm.
	if equivalentGroup != "" {
		switch yes, err := auth.IsMember(ctx, equivalentGroup); {
		case err != nil:
			return false, fmt.Errorf("failed to perform membership check for group %q: %w", equivalentGroup, err)
		case yes:
			return true, nil
		}
	}
	realm := realms.Join(project, realms.RootRealm)
	switch yes, err := auth.HasPermission(ctx, perm, realm, nil); {
	case err != nil:
		return false, fmt.Errorf("failed to check permission %s in realm %q: %w", perm, realm, err)
	default:
		return yes, nil
	}
}
