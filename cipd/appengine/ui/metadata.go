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

package ui

import (
	"context"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

// prefixMetadataBlock is passed to the templates as Metadata arg.
type prefixMetadataBlock struct {
	// CanView is true if the caller is able to see all prefix metadata.
	CanView bool

	// ACLs is per-principal acls sorted by prefix length and role.
	//
	// Populated only if CanView is true.
	ACLs []*metadataACL

	// CallerRoles is roles of the current caller.
	//
	// Populated only if CanView is false.
	CallerRoles []string
}

type metadataACL struct {
	RolePb     repopb.Role // original role enum, for sorting
	Role       string      // e.g. "Reader"
	Who        string      // either an email or a group name
	WhoHref    string      // for groups, link to a group definition
	Group      bool        // if true, this entry is a group
	Missing    bool        // if true, this entry refers to a missing group
	Prefix     string      // via what prefix this role is granted
	PrefixHref string      // link to the corresponding prefix page
}

// fetchPrefixMetadata fetches and formats for UI metadata of the given prefix.
//
// It recognizes PermissionDenied errors and falls back to only displaying what
// roles the caller has instead of the full metadata.
func fetchPrefixMetadata(ctx context.Context, pfx string) (*prefixMetadataBlock, error) {
	meta, err := state(ctx).services.PublicRepo.GetInheritedPrefixMetadata(ctx, &repopb.PrefixRequest{
		Prefix: pfx,
	})
	switch status.Code(err) {
	case codes.OK:
		break // handled below
	case codes.PermissionDenied:
		return fetchCallerRoles(ctx, pfx)
	default:
		return nil, err
	}

	db := auth.GetState(ctx).DB()

	// Grab URL of an auth server with the groups, if available.
	groupsURL := ""
	if url, err := db.GetAuthServiceURL(ctx); err == nil {
		groupsURL = url + "/auth/groups/"
	}

	// Collect all groups mentioned by the ACL to flag unknown ones.
	groups := stringset.New(0)

	out := &prefixMetadataBlock{CanView: true}
	for _, m := range meta.PerPrefixMetadata {
		for _, a := range m.Acls {
			role := strings.Title(strings.ToLower(a.Role.String()))

			prefix := m.Prefix
			if prefix == "" {
				prefix = "[root]"
			}

			for _, p := range a.Principals {
				whoHref := ""
				group := false
				switch {
				case strings.HasPrefix(p, "group:"):
					p = strings.TrimPrefix(p, "group:")
					if groupsURL != "" {
						whoHref = groupsURL + p
					}
					groups.Add(p)
					group = true
				case p == string(identity.AnonymousIdentity):
					p = "anonymous"
				default:
					p = strings.TrimPrefix(p, "user:")
				}

				out.ACLs = append(out.ACLs, &metadataACL{
					RolePb:     a.Role,
					Role:       role,
					Who:        p,
					WhoHref:    whoHref,
					Group:      group,
					Prefix:     prefix,
					PrefixHref: listingPageURL(m.Prefix, ""),
				})
			}
		}
	}

	sort.Slice(out.ACLs, func(i, j int) bool {
		l, r := out.ACLs[i], out.ACLs[j]
		if l.RolePb != r.RolePb {
			return l.RolePb > r.RolePb // "stronger" role (e.g. OWNERS) first
		}
		if l.Prefix != r.Prefix {
			return l.Prefix > r.Prefix // longer prefix first
		}
		return l.Who < r.Who // alphabetically
	})

	// If the caller is allowed to see actual contents of groups, flag groups that
	// do not exist.
	if groups.Len() > 0 {
		switch yes, err := db.IsMember(ctx, auth.CurrentIdentity(ctx), []string{authdb.AuthServiceAccessGroup}); {
		case yes:
			known, err := db.FilterKnownGroups(ctx, groups.ToSlice())
			if err != nil {
				return nil, err
			}
			knownSet := stringset.NewFromSlice(known...)
			for _, acl := range out.ACLs {
				acl.Missing = acl.Group && !knownSet.Has(acl.Who)
			}
		case err != nil:
			return nil, err
		}
	}

	return out, nil
}

func fetchCallerRoles(ctx context.Context, pfx string) (*prefixMetadataBlock, error) {
	roles, err := state(ctx).services.PublicRepo.GetRolesInPrefix(ctx, &repopb.PrefixRequest{
		Prefix: pfx,
	})
	if err != nil {
		return nil, err
	}
	out := &prefixMetadataBlock{
		CanView:     false,
		CallerRoles: make([]string, len(roles.Roles)),
	}
	for i, r := range roles.Roles {
		out.CallerRoles[i] = strings.Title(strings.ToLower(r.Role.String()))
	}
	return out, nil
}
