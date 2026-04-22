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

package permissions

import (
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/configspb"
)

// PermissionsDB is a representation of all defined roles, permissions and
// implicit bindings.
//
// This will be generated from permissions.cfg, once constructed this must
// be treated as immutable.
//
// Revision property follows the rule that if two DB's have the same revision
// than they are identical, but if they don't have the same revision it does
// not necessarily mean they are not identical.
type PermissionsDB struct {
	// Rev is the revision of this permissions DB.
	Rev string
	// Permissions is a map of permission name -> *protocol.Permission.
	Permissions map[string]*protocol.Permission
	// Roles is a mapping of role name to Role.
	Roles map[string]*Role
	// ImplicitRootBindings generates implicit bindings for the given project.
	ImplicitRootBindings func(projID, projAcc string) []*realmsconf.Binding
}

// Role represents a single role with the fully expanded permissions set.
type Role struct {
	// Name is the full name for this role.
	Name string
	// Permissions are all permissions that this role expands into.
	Permissions stringset.Set
	// Includes is a list of roles directly included by this role.
	Includes []string
	// RelevantAttributes is a union of attributes of all included permissions.
	RelevantAttributes stringset.Set
}

// NewPermissionsDB constructs a new instance of PermissionsDB from a given
// validated permissions.cfg.
func NewPermissionsDB(permissionscfg *configspb.PermissionsConfig, meta *config.Meta) *PermissionsDB {
	rev := "config-without-metadata"
	if meta != nil {
		rev = fmt.Sprintf("%s:%s", meta.Path, meta.Revision)
	}

	permissions := make(map[string]*protocol.Permission, len(permissionscfg.GetPermission()))
	for _, perm := range permissionscfg.GetPermission() {
		perm = proto.CloneOf(perm)
		slices.Sort(perm.Attributes)
		permissions[perm.Name] = perm
	}

	roles := make(map[string]*Role, len(permissionscfg.GetRole()))
	for _, role := range permissionscfg.GetRole() {
		perms := make(stringset.Set, len(role.Permissions))
		for _, perm := range role.Permissions {
			perms.Add(perm.Name)
		}
		roles[role.Name] = &Role{
			Name:        role.Name,
			Permissions: perms,
			Includes:    role.Includes,
		}
	}

	// Recursively expand all "includes" relations.
	expanded := make(map[string]bool, len(roles))
	var expandRole func(role *Role)
	expandRole = func(role *Role) {
		if expanded[role.Name] {
			return
		}
		expanded[role.Name] = true
		for _, included := range role.Includes {
			expandRole(roles[included])
			for perm := range roles[included].Permissions {
				role.Permissions.Add(perm)
			}
		}
	}
	for _, role := range roles {
		expandRole(role)
	}

	// Collect RelevantAttributes sets.
	for _, role := range roles {
		for perm := range role.Permissions {
			if attrs := permissions[perm].GetAttributes(); len(attrs) > 0 {
				if role.RelevantAttributes == nil {
					role.RelevantAttributes = make(stringset.Set, len(attrs))
				}
				role.RelevantAttributes.AddAll(attrs)
			}
		}
	}

	return &PermissionsDB{
		Rev:         rev,
		Permissions: permissions,
		Roles:       roles,
		ImplicitRootBindings: func(projID, projAcc string) []*realmsconf.Binding {
			bindings := []*realmsconf.Binding{
				{
					Role:       "role/luci.internal.system",
					Principals: []string{fmt.Sprintf("project:%s", projID)},
				},
				{
					Role:       "role/luci.internal.buildbucket.reader",
					Principals: []string{"group:buildbucket-internal-readers"},
				},
				{
					Role:       "role/luci.internal.resultdb.reader",
					Principals: []string{"group:resultdb-internal-readers"},
				},
				{
					Role:       "role/luci.internal.resultdb.invocationSubmittedSetter",
					Principals: []string{"group:resultdb-internal-invocation-submitters"},
				},
			}
			if projAcc != "" {
				bindings = append(bindings, &realmsconf.Binding{
					Role:       "role/luci.internal.projectScopedAccount",
					Principals: []string{fmt.Sprintf("user:%s", projAcc)},
				})
			}
			return bindings
		},
	}
}

// IsAttributeRelevantForRole returns true if the given attribute is referenced
// by at least one permission the given role expands into.
func (db *PermissionsDB) IsAttributeRelevantForRole(attr, role string) bool {
	if r, ok := db.Roles[role]; ok {
		return r.RelevantAttributes.Has(attr)
	}
	return false
}
