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

package repo

import (
	"context"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
)

// impliedRoles defines what roles are "inherited" by other roles, e.g.
// WRITERs are automatically READERs, so hasRole(..., READER) should return true
// for WRITERs too.
//
// The format is "role -> {role itself} + set of roles implied by it, perhaps
// indirectly".
//
// If a role is missing from this map, it assumed to not be implying any roles.
var impliedRoles = map[repopb.Role][]repopb.Role{
	repopb.Role_READER: {repopb.Role_READER},
	repopb.Role_WRITER: {repopb.Role_WRITER, repopb.Role_READER},
	repopb.Role_OWNER:  {repopb.Role_OWNER, repopb.Role_WRITER, repopb.Role_READER},
}

// impliedRolesRev is reverse of impliedRoles mapping.
//
// The format is "role -> {role itself} + set of roles that inherit it, perhaps
// indirectly".
//
// If a role is missing from this map, it assumed to not be inherited by
// anything.
var impliedRolesRev = map[repopb.Role]map[repopb.Role]struct{}{
	repopb.Role_READER: roleSet(repopb.Role_READER, repopb.Role_WRITER, repopb.Role_OWNER),
	repopb.Role_WRITER: roleSet(repopb.Role_WRITER, repopb.Role_OWNER),
	repopb.Role_OWNER:  roleSet(repopb.Role_OWNER),
}

func roleSet(roles ...repopb.Role) map[repopb.Role]struct{} {
	m := make(map[repopb.Role]struct{}, len(roles))
	for _, r := range roles {
		m[r] = struct{}{}
	}
	return m
}

// isRoleAllowed checks whether the current caller is allowed to have the
// given role. This policy is independent from ACLs and will override any
// other policies.
func isRoleAllowed(ident identity.Identity, cfg *prefixcfg.Entry, role repopb.Role) bool {
	if role == repopb.Role_READER {
		return true
	}

	if len(cfg.AllowWritersFromRegexp) == 0 {
		return true
	}

	for _, re := range cfg.AllowWritersFromRegexp {
		if re.MatchString(ident.Email()) {
			return true
		}
	}

	return false
}

type rolePolicy string

const (
	rolePolicyConfig rolePolicy = "policy"
	rolePolicyACL    rolePolicy = "acl"
)

// hasRole checks whether the current caller has the given role in any of the
// supplied PrefixMetadata objects.
//
// It understands the role inheritance defined by impliedRoles map.
//
// 'cfg' is the is the prefix config of the leaf prefix being checked.
//
// 'metas' is metadata for some prefix and all parent prefixes. It is expected
// to be ordered by the prefix length (shortest first). Ordering is not really
// used now, but it may change in the future.
//
// Returns the result and the policy source. Result only contains transient errors.
func hasRole(ctx context.Context, cfg *prefixcfg.Entry, metas []*repopb.PrefixMetadata, role repopb.Role) (bool, rolePolicy, error) {
	ident := auth.CurrentIdentity(ctx)
	if !isRoleAllowed(ident, cfg, role) {
		return false, rolePolicyConfig, nil
	}

	caller := string(ident) // e.g. "user:abc@example.com"

	// E.g. if 'role' is READER, 'roles' will be {READER, WRITER, OWNER}.
	roles := impliedRolesRev[role]
	if roles == nil {
		roles = roleSet(role)
	}

	// Enumerate the set of principals that have any of the requested roles in any
	// of the prefixes. Exit early if hitting the direct match, otherwise proceed
	// to more expensive group membership checks. Note that we don't use isInACL
	// here because we want to postpone all group checks until the very end,
	// checking memberships in all groups mentioned in 'metas' at once.
	groups := stringset.New(10) // 10 is picked arbitrarily
	for _, meta := range metas {
		for _, acl := range meta.Acls {
			if _, ok := roles[acl.Role]; !ok {
				continue // not the role we are interested in
			}
			for _, p := range acl.Principals {
				if p == caller {
					return true, rolePolicyACL, nil // the caller was specified in ACLs explicitly
				}
				// Is this a reference to a group?
				if s := strings.SplitN(p, ":", 2); len(s) == 2 && s[0] == "group" {
					groups.Add(s[1])
				}
			}
		}
	}

	yes, err := auth.IsMember(ctx, groups.ToSlice()...)
	if err != nil {
		return false, "", errors.Fmt("failed to check group memberships when checking ACLs for role %s: %w", role, err)
	}
	return yes, rolePolicyACL, nil
}

// rolesInPrefix returns a union of roles `ident` has in given supplied
// PrefixMetadata objects.
//
// It understands the role inheritance defined by impliedRoles map.
//
// Returns only transient errors.
func rolesInPrefix(ctx context.Context, ident identity.Identity, cfg *prefixcfg.Entry, metas []*repopb.PrefixMetadata) ([]repopb.Role, error) {
	roles := roleSet()
	for _, meta := range metas {
		for _, acl := range meta.Acls {
			if _, ok := roles[acl.Role]; ok {
				continue // seen this role already
			}
			switch yes, err := isInACL(ctx, ident, acl); {
			case err != nil:
				return nil, err
			case yes:
				// Add acl.Role and all roles implied by it to 'roles' set.
				for _, r := range impliedRoles[acl.Role] {
					if isRoleAllowed(ident, cfg, r) {
						roles[r] = struct{}{}
					}
				}
			}
		}
	}

	// Arrange the result in the order of Role enum definition.
	out := make([]repopb.Role, 0, len(roles))
	for r := repopb.Role_READER; r <= repopb.Role_OWNER; r++ {
		if _, ok := roles[r]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// isInACL is true if `ident` is in the given access control list.
//
// Most callers will use auth.CurrentIdentity() for this value.
func isInACL(ctx context.Context, ident identity.Identity, acl *repopb.PrefixMetadata_ACL) (bool, error) {
	var groups []string
	for _, p := range acl.Principals {
		if p == string(ident) {
			return true, nil // the identity was specified in ACLs explicitly
		}
		if s := strings.SplitN(p, ":", 2); len(s) == 2 && s[0] == "group" {
			groups = append(groups, s[1])
		}
	}

	// We don't use auth.IsMember because we want to check for `ident` rather than
	// for the current caller.
	var yes bool
	var err error
	if s := auth.GetState(ctx); s != nil {
		yes, err = s.DB().IsMember(ctx, ident, groups)
	} else {
		err = auth.ErrNotConfigured
	}
	if err != nil {
		return false, errors.Fmt("failed to check group memberships when checking ACLs: %w", err)
	}
	return yes, nil
}
