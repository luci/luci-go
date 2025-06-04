// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bqexport

import (
	"context"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config"

	"go.chromium.org/luci/auth_service/api/bqpb"
	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/permissionscfg"
	"go.chromium.org/luci/auth_service/internal/realmsinternals"
)

type ExpandedRole struct {
	Name        string
	Subroles    stringset.Set
	Permissions stringset.Set
	ViewURL     string
}

type RoleSet map[string]*ExpandedRole

// Absorb extends this role by all permissions and subroles granted to other.
func (er *ExpandedRole) Absorb(other *ExpandedRole) {
	er.Permissions.AddAll(other.Permissions.ToSlice())
	er.Subroles.AddAll(other.Subroles.ToSlice())
	er.Subroles.Add(other.Name)
}

// analyzePermissionsCfg analyzes the given config for all globally defined roles.
func analyzePermissionsCfg(ctx context.Context,
	cfg *configspb.PermissionsConfig, cfgMeta *config.Meta) (stringset.Set, RoleSet) {
	cfgRoles := cfg.GetRole()
	permissions := stringset.Set{}
	roleSet := make(map[string]*ExpandedRole, len(cfgRoles))
	for _, cfgRole := range cfgRoles {
		name := cfgRole.GetName()
		r := &ExpandedRole{
			Name:        name,
			Subroles:    stringset.Set{},
			Permissions: stringset.Set{},
			ViewURL:     cfgMeta.ViewURL,
		}
		for _, p := range cfgRole.GetPermissions() {
			perm := p.GetName()
			permissions.Add(perm)
			r.Permissions.Add(perm)
		}
		roleSet[name] = r
	}

	// Handle subroles now that all roles are in the map.
	for _, cfgRole := range cfgRoles {
		for _, subroleName := range cfgRole.GetIncludes() {
			subrole, ok := roleSet[subroleName]
			if !ok {
				logging.Warningf(ctx, "skipping role absorption - missing role %q", subroleName)
				continue
			}
			roleSet[cfgRole.GetName()].Absorb(subrole)
		}
	}

	return permissions, roleSet
}

// analyzeRealmsCfg analyzes the given config for all defined custom roles.
func analyzeRealmsCfg(ctx context.Context, cfg *config.Config,
	perms stringset.Set, globalRoles RoleSet) (RoleSet, error) {
	parsed := &realmsconf.RealmsCfg{}
	if err := prototext.Unmarshal([]byte(cfg.Content), parsed); err != nil {
		return nil, errors.Fmt("failed to unmarshal config body for realms: %w", err)
	}

	customRoles := parsed.GetCustomRoles()
	roleSet := make(map[string]*ExpandedRole, len(customRoles))
	for _, customRole := range customRoles {
		name := customRole.GetName()
		r := &ExpandedRole{
			Name:        name,
			Subroles:    stringset.Set{},
			Permissions: stringset.Set{},
			ViewURL:     cfg.ViewURL,
		}
		rolePerms := customRole.GetPermissions()
		if !perms.HasAll(rolePerms...) {
			logging.Warningf(ctx, "not all permissions for %s>>%s are known",
				cfg.ViewURL, name)
		}
		r.Permissions.AddAll(rolePerms)
		roleSet[name] = r
	}

	// Handle subroles now that all custom roles are in the map.
	for _, customRole := range customRoles {
		for _, subroleName := range customRole.GetExtends() {
			// Check global roles first.
			subrole, ok := globalRoles[subroleName]
			if !ok {
				// Check this config's custom roles.
				subrole, ok = roleSet[subroleName]
				if !ok {
					logging.Warningf(ctx, "skipping role absorption - missing role %q", subroleName)
					continue
				}
			}
			roleSet[customRole.GetName()].Absorb(subrole)
		}
	}

	return roleSet, nil
}

func collateLatestRoles(ctx context.Context, ts *timestamppb.Timestamp) ([]*bqpb.RoleRow, error) {
	permsCfg, permsMeta, err := permissionscfg.GetWithMetadata(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to get permissions.cfg: %w", err)
	}

	latest, err := realmsinternals.FetchLatestRealmsConfigs(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to fetch latest realms: %w", err)
	}

	count := 0

	// Get the global permissions and global roles defined in permissions.cfg.
	perms, globalRoles := analyzePermissionsCfg(ctx, permsCfg, permsMeta)
	roleSets := make([]RoleSet, 0, len(latest)+1)
	roleSets = append(roleSets, globalRoles)
	count += len(globalRoles)

	// Get the custom roles defined in realms configs.
	for _, cfg := range latest {
		rs, err := analyzeRealmsCfg(ctx, cfg, perms, globalRoles)
		if err != nil {
			return nil, err
		}
		roleSets = append(roleSets, rs)
		count += len(rs)
	}

	// Convert each to a bqpb.RoleRow.
	out := make([]*bqpb.RoleRow, 0, count)
	for _, rs := range roleSets {
		for _, role := range rs {
			out = append(out, &bqpb.RoleRow{
				Name:        role.Name,
				Subroles:    role.Subroles.ToSortedSlice(),
				Permissions: role.Permissions.ToSortedSlice(),
				Url:         role.ViewURL,
				ExportedAt:  ts,
			})
		}
	}

	return out, nil
}
