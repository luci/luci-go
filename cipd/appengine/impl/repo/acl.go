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
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// impliedRoles defines what roles are "inherited" by other roles, e.g.
// WRITERs are automatically READERs, so hasRole(..., READER) should return true
// for WRITERs too.
//
// The format is "role -> {role itself} + set of roles that inherit it, perhaps
// indirectly".
//
// If a role is missing from this map, it assumed to not be inherited by
// anything.
var impliedRoles = map[api.Role]map[api.Role]struct{}{
	api.Role_READER: roleSet(api.Role_READER, api.Role_WRITER, api.Role_OWNER),
	api.Role_WRITER: roleSet(api.Role_WRITER, api.Role_OWNER),
	api.Role_OWNER:  roleSet(api.Role_OWNER),
}

func roleSet(roles ...api.Role) map[api.Role]struct{} {
	m := make(map[api.Role]struct{}, len(roles))
	for _, r := range roles {
		m[r] = struct{}{}
	}
	return m
}

// hasRole checks whether the current caller has the given role in any of the
// supplied PrefixMetadata objects.
//
// It understands the role inheritance defined by impliedRoles map.
//
// 'metas' is metadata for some prefix and all parent prefixes. It is expected
// to be ordered by the prefix length (shortest first). Ordering is not really
// used now, but it may change in the future.
//
// Returns only transient errors.
func hasRole(c context.Context, metas []*api.PrefixMetadata, role api.Role) (bool, error) {
	caller := string(auth.CurrentIdentity(c)) // e.g. "user:abc@example.com"

	// E.g. if 'role' is READER, 'roles' will be {READER, WRITER, OWNER}.
	roles := impliedRoles[role]
	if roles == nil {
		roles = roleSet(role)
	}

	// Enumerate the set of principals that have any of the requested roles in any
	// of the prefixes. Exit early if hitting the direct match, otherwise proceed
	// to more expensive group membership checks.
	groups := stringset.New(10) // 10 is picked arbitrarily
	for _, meta := range metas {
		for _, acl := range meta.Acls {
			if _, ok := roles[acl.Role]; !ok {
				continue // not the role we are interested in
			}
			for _, p := range acl.Principals {
				if p == caller {
					return true, nil // the caller was specified in ACLs explicitly
				}
				// Is this a reference to a group?
				if s := strings.SplitN(p, ":", 2); len(s) == 2 && s[0] == "group" {
					groups.Add(s[1])
				}
			}
		}
	}

	// Administrators group has implicit permissions to do everything everywhere.
	//
	// TODO(vadimsh): Get rid of this. It's not entirely clear how to deal with
	// ACL for the root of the package namespace then, e.g. how to define who can
	// create root prefixes. Maybe we need a special RPC or special non-standard
	// non-inherited ACL just for dealing with the root.
	groups.Add("administrators")

	yes, err := auth.IsMember(c, groups.ToSlice()...)
	if err != nil {
		return false, errors.Annotate(err, "failed to check group memberships when checking ACLs for role %s", role).Err()
	}
	return yes, nil
}
