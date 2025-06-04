// Copyright 2024 The LUCI Authors.
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

package perms

import (
	"context"
	"fmt"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/tree_status/internal/config"
)

const treeStatusAccessGroup = "luci-tree-status-access"
const treeStatusWriteAccessGroup = "luci-tree-status-writers"

// treeStatusAuditAccessGroup is the group which contains people authorised
// to see the details of the user who created entities.
// In go/cria, treeStatusAuditAccessGroup is a sub-group of treeStatusAccessGroup.
const treeStatusAuditAccessGroup = "luci-tree-status-audit-access"

// HasGetStatusLimitedPermission returns if the user can get the status (without PII) of a tree.
// If the user has no permission, an error message will also be returned.
// err will be returned if there is some error during the ACL check.
func HasGetStatusLimitedPermission(ctx context.Context, treeName string) (allowed bool, message string, err error) {
	return hasAccess(ctx, treeName, treeStatusAccessGroup, PermGetStatusLimited)
}

// HasListStatusLimitedPermission returns if the user can list the statuses (without PII) of a tree.
// If the user has no permission, an error message will also be returned.
// err will be returned if there is some error during the ACL check.
func HasListStatusLimitedPermission(ctx context.Context, treeName string) (allowed bool, message string, err error) {
	return hasAccess(ctx, treeName, treeStatusAccessGroup, PermListStatusLimited)
}

// HasGetStatusPermission returns if the user can get the status (with PII) of a tree.
// If the user has no permission, an error message will also be returned.
// err will be returned if there is some error during the ACL check.
func HasGetStatusPermission(ctx context.Context, treeName string) (allowed bool, message string, err error) {
	return hasAccess(ctx, treeName, treeStatusAuditAccessGroup, PermGetStatus)
}

// HasListStatusPermission returns if the user can list the statuses (without PII) of a tree.
// If the user has no permission, an error message will also be returned.
// err will be returned if there is some error during the ACL check.
func HasListStatusPermission(ctx context.Context, treeName string) (allowed bool, message string, err error) {
	return hasAccess(ctx, treeName, treeStatusAuditAccessGroup, PermListStatus)
}

// HasCreateStatusPermission returns if the user can create a status in a tree.
// If the user has no permission, an error message will also be returned.
// err will be returned if there is some error during the ACL check.
func HasCreateStatusPermission(ctx context.Context, treeName string) (allowed bool, message string, err error) {
	return hasAccess(ctx, treeName, treeStatusWriteAccessGroup, PermCreateStatus)
}

// HasQueryTreesPermission returns if the user can query trees.
// If the user has no permission, an error message will also be returned.
// err will be returned if there is some error during the ACL check.
func HasQueryTreesPermission(ctx context.Context, treeName string) (allowed bool, message string, err error) {
	return hasAccess(ctx, treeName, treeStatusAccessGroup, PermListTree)
}

// HasGetTreePermission returns if the user can get a tree.
// If the user has no permission, an error message will also be returned.
// err will be returned if there is some error during the ACL check.
func HasGetTreePermission(ctx context.Context, treeName string) (allowed bool, message string, err error) {
	return hasAccess(ctx, treeName, treeStatusAccessGroup, PermGetTree)
}

// hasAccess checks if the user has access to a tree.
// If the tree uses default ACLs, the access will be checked against the go/cria group.
// Otherwise, the permission will be check against the subrealm of the primary project.
//
// In case of GetStatus and ListStatus, then the user needs to be in treeStatusAuditAccessGroup, even
// if the tree uses realm-based ACL. This is a safe-guard mechanism in case realm permission
// was not configured correctly and allows non-Googlers to see PII.
//
// If the user has no permission, an detailed message will also be returned.
// err will be returned if there are some errors during the ACL check.
func hasAccess(ctx context.Context, treeName string, criaGroup string, permission realms.Permission) (allowed bool, message string, err error) {
	treeCfg, err := config.GetTreeConfig(ctx, treeName)
	if err != nil {
		if errors.Is(err, config.ErrNotFoundTreeConfig) {
			return false, "tree has not been configured, see go/luci-guide-tree-status#creating-a-tree-and-setting-up-permissions for more information", nil
		}
		return false, "", err
	}

	// Check against the CRIA group.
	if treeCfg.UseDefaultAcls {
		logging.Infof(ctx, "Tree is using default ACLs")
		return checkCriaGroup(ctx, criaGroup)
	}

	// The tree is using realm-based ACLs.
	logging.Infof(ctx, "Tree is using realm-based ACLs")

	// Get the primary project of the tree.
	// The config validation should ensure that there should be at least 1 project
	// when UseDefaultAcls == false, but we check here again just for safe.
	if len(treeCfg.Projects) == 0 {
		return false, "projects in tree has not been configured", nil
	}
	primaryProject := treeCfg.Projects[0]
	subrealm := treeCfg.Subrealm
	if subrealm == "" {
		subrealm = realms.ProjectRealm
	}
	allowed, err = HasProjectPermission(ctx, primaryProject, subrealm, permission)
	if err != nil {
		return false, "", err
	}
	if !allowed {
		return false, "user does not have permission to perform this action", nil
	}

	// Special case: For GetStatus and ListStatus, we also require the user to be in
	// treeStatusAuditAccessGroup.
	if permission == PermGetStatus || permission == PermListStatus {
		return checkCriaGroup(ctx, treeStatusAuditAccessGroup)
	}
	return true, "", nil
}

func checkCriaGroup(ctx context.Context, criaGroup string) (bool, string, error) {
	switch yes, err := auth.IsMember(ctx, criaGroup); {
	case err != nil:
		return false, "", errors.Fmt("failed to check ACL: %w", err)
	case !yes:
		if auth.CurrentIdentity(ctx).Kind() == identity.Anonymous {
			return false, "please log in for access", nil
		}
		return false, fmt.Sprintf("user is not a member of group %q", criaGroup), nil
	default:
		return true, "", nil
	}
}

// HasProjectPermission returns if the caller has the given permission in
// the subrealm of the given project. This method only returns an error
// if there is some AuthDB issue.
func HasProjectPermission(ctx context.Context, project string, subrealm string, permission realms.Permission) (bool, error) {
	realm := realms.Join(project, subrealm)
	switch allowed, err := auth.HasPermission(ctx, permission, realm, nil); {
	case err != nil:
		return false, err
	case !allowed:
		return false, nil
	}
	return true, nil
}
