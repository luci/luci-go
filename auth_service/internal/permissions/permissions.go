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

	"go.chromium.org/luci/common/data/stringset"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/configspb"
)

// PermissionsDB is a representation of all defined roles, permissions
// and implicit bindings.
//
// This will be generated from permissions.cfg, once constructed this must
// be treated as immutable.
//
// Revision property follows the rule that if two DB's have the same revision
// than they are identical, but if they don't have the same revision it does
// not necessarily mean they are not identical.
type PermissionsDB struct {
	// Rev is the revision of this permissionDB
	Rev string

	// Permissions is a map of Permissions str -> *protocol.Permission
	Permissions map[string]*protocol.Permission

	// Roles is a mapping of RoleName to Role.
	Roles map[string]*Role

	// attributes is a set with attribute names allowed in conditions
	attributes stringset.Set

	// func(projID) -> []*realmsconf.Binding
	ImplicitRootBindings func(string) []*realmsconf.Binding
}

// Role represents a single role, containing the role
// name and the permissions associated with this role.
type Role struct {
	// Name is the full name for this role
	Name string

	// Permissions contains all the permission strings for this
	// role
	Permissions stringset.Set
}

// NewPermissionsDB constructs a new instance of PermissionsDB from a given permissions.cfg.
func NewPermissionsDB(permissionscfg *configspb.PermissionsConfig, meta *config.Meta) *PermissionsDB {
	rev := "config-without-metadata"
	if meta != nil {
		rev = fmt.Sprintf("%s:%s", meta.Path, meta.Revision)
	}

	permissionsDB := &PermissionsDB{
		Rev:         rev,
		Permissions: make(map[string]*protocol.Permission),
		Roles:       make(map[string]*Role),
	}

	for _, role := range permissionscfg.GetRole() {
		permissionsDB.Roles[role.GetName()] = &Role{role.GetName(), stringset.Set{}}
		for _, perm := range role.GetPermissions() {
			permissionsDB.Permissions[perm.GetName()] = perm
			permissionsDB.Roles[role.GetName()].Permissions.Add(perm.GetName())
		}
	}

	// Expand includes after all values in map
	for _, role := range permissionscfg.GetRole() {
		for _, inc := range role.GetIncludes() {
			permissionsDB.Roles[role.GetName()].Permissions = permissionsDB.Roles[role.GetName()].Permissions.Union(permissionsDB.Roles[inc].Permissions)
		}
	}
	permissionsDB.attributes = stringset.NewFromSlice(permissionscfg.GetAttribute()...)
	permissionsDB.ImplicitRootBindings = func(projID string) []*realmsconf.Binding {
		return []*realmsconf.Binding{
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
	}
	return permissionsDB
}

func (db *PermissionsDB) HasAttribute(attr string) bool {
	return db.attributes.Has(attr)
}
