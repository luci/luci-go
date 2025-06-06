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

package cipd

import (
	"time"

	"go.chromium.org/luci/common/errors"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// Helper structs and functions for working with package ACLs.

// PackageACLChangeAction defines a flavor of PackageACLChange.
//
// Used by ModifyACL.
type PackageACLChangeAction string

const (
	// GrantRole is used in PackageACLChange to request a role to be granted.
	GrantRole PackageACLChangeAction = "GRANT"
	// RevokeRole is used in PackageACLChange to request a role to be revoked.
	RevokeRole PackageACLChangeAction = "REVOKE"
)

// PackageACL is per package path per role access control list that is a part of
// larger overall ACL: ACL for package "a/b/c" is a union of PackageACLs for "a"
// "a/b" and "a/b/c".
type PackageACL struct {
	// PackagePath is a package subpath this ACL is defined for.
	PackagePath string `json:"package_path"`
	// Role is a role that listed users have, e.g. 'READER', 'WRITER', ...
	Role string `json:"role"`
	// Principals list users and groups granted the role.
	Principals []string `json:"principals"`
	// ModifiedBy specifies who modified the list the last time.
	ModifiedBy string `json:"modified_by"`
	// ModifiedTs is a timestamp when the list was modified the last time.
	ModifiedTs UnixTime `json:"modified_ts"`
}

// PackageACLChange is a mutation to some package ACL.
type PackageACLChange struct {
	// Action defines what action to perform: GrantRole or RevokeRole.
	Action PackageACLChangeAction
	// Role to grant or revoke to a user or group, see Role enum repo.proto.
	Role string
	// Principal is a user or a group to grant or revoke a role for.
	Principal string
}

// prefixMetadataToACLs extracts ACLs for a prefix and all its parent prefixes
// from the prefix's metadata.
func prefixMetadataToACLs(m *api.InheritedPrefixMetadata) (out []PackageACL) {
	for _, p := range m.PerPrefixMetadata {
		var acls []PackageACL
		for _, acl := range p.Acls {
			role := acl.Role.String()
			found := false
			for i, existing := range acls {
				if existing.Role == role {
					acls[i].Principals = append(acls[i].Principals, acl.Principals...)
					found = true
					break
				}
			}
			if !found {
				var t time.Time
				if p.UpdateTime.IsValid() {
					t = p.UpdateTime.AsTime()
				}
				acls = append(acls, PackageACL{
					PackagePath: p.Prefix,
					Role:        role,
					Principals:  acl.Principals,
					ModifiedBy:  p.UpdateUser,
					ModifiedTs:  UnixTime(t),
				})
			}
		}
		out = append(out, acls...)
	}
	return
}

// mutateACLs applies changes to ACLs in the prefix metadata.
//
// Returns true if made some changes (even if ultimately failed), false if not.
func mutateACLs(meta *api.PrefixMetadata, changes []PackageACLChange) (dirty bool, err error) {
	for _, ch := range changes {
		role := api.Role(api.Role_value[ch.Role])
		if role == 0 {
			return dirty, cipderr.BadArgument.Apply(errors.Fmt("unrecognized role %q, not in the API definition", ch.Role))
		}
		changed := false
		switch ch.Action {
		case GrantRole:
			changed = grantRole(meta, role, ch.Principal)
		case RevokeRole:
			changed = revokeRole(meta, role, ch.Principal)
		default:
			return dirty, cipderr.BadArgument.Apply(errors.Fmt("unrecognized PackageACLChangeAction %q", ch.Action))
		}
		dirty = dirty || changed
	}
	return
}

func grantRole(m *api.PrefixMetadata, role api.Role, principal string) bool {
	var roleACL *api.PrefixMetadata_ACL
	for _, acl := range m.Acls {
		if acl.Role != role {
			continue
		}
		for _, p := range acl.Principals {
			if p == principal {
				return false // already have it
			}
		}
		roleACL = acl
	}

	if roleACL != nil {
		// Append to the existing ACL.
		roleACL.Principals = append(roleACL.Principals, principal)
	} else {
		// Add new ACL for this role, this is the first one.
		m.Acls = append(m.Acls, &api.PrefixMetadata_ACL{
			Role:       role,
			Principals: []string{principal},
		})
	}

	return true
}

func revokeRole(m *api.PrefixMetadata, role api.Role, principal string) bool {
	dirty := false
	for _, acl := range m.Acls {
		if acl.Role != role {
			continue
		}
		filtered := acl.Principals[:0]
		for _, p := range acl.Principals {
			if p != principal {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) != len(acl.Principals) {
			acl.Principals = filtered
			dirty = true
		}
	}

	if !dirty {
		return false
	}

	// Kick out empty ACL entries.
	acls := m.Acls[:0]
	for _, acl := range m.Acls {
		if len(acl.Principals) != 0 {
			acls = append(acls, acl)
		}
	}
	if len(acls) == 0 {
		m.Acls = nil
	} else {
		m.Acls = acls
	}
	return true
}
