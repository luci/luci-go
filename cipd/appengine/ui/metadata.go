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
	"sort"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl"
)

// metadataBlock is passed to the templates as Metadata arg.
type metadataBlock struct {
	// CanView is true if the caller is able to see all prefix metadata.
	CanView bool

	// ACLs is per-principal acls sorted by prefix length and role.
	//
	// Populated only if CanView is true.
	ACLs []metadataACL

	// CallerRoles is roles of the current caller.
	//
	// Populated only if CanView is false.
	CallerRoles []string
}

type metadataACL struct {
	RolePb     api.Role // original role enum, for sorting
	Role       string   // e.g "Reader"
	Who        string   // either an email or a group name
	WhoHref    string   // for groups, link to a group definition
	Prefix     string   // via what prefix this role is granted
	PrefixHref string   // link to the corresponding prefix page
}

// fetchMetadata fetches and formats for UI metadata of the given prefix.
//
// It recognizes PermissionDenied errors and falls back to only displaying what
// roles the caller has instead of the full metadata.
func fetchMetadata(c context.Context, pfx string) (*metadataBlock, error) {
	meta, err := impl.PublicRepo.GetInheritedPrefixMetadata(c, &api.PrefixRequest{
		Prefix: pfx,
	})
	switch grpc.Code(err) {
	case codes.OK:
		break // handled below
	case codes.PermissionDenied:
		return fetchCallerRoles(c, pfx)
	default:
		return nil, err
	}

	// Grab URL of an auth server with the groups, if available.
	groupsURL := ""
	if url, err := auth.GetState(c).DB().GetAuthServiceURL(c); err == nil {
		groupsURL = url + "/auth/groups/"
	}

	out := &metadataBlock{CanView: true}
	for _, m := range meta.PerPrefixMetadata {
		for _, a := range m.Acls {
			role := strings.Title(strings.ToLower(a.Role.String()))

			prefix := m.Prefix
			if prefix == "" {
				prefix = "[root]"
			}

			for _, p := range a.Principals {
				whoHref := ""
				switch {
				case strings.HasPrefix(p, "group:"):
					p = strings.TrimPrefix(p, "group:")
					if groupsURL != "" {
						whoHref = groupsURL + p
					}
				case p == string(identity.AnonymousIdentity):
					p = "anonymous"
				default:
					p = strings.TrimPrefix(p, "user:")
				}

				out.ACLs = append(out.ACLs, metadataACL{
					RolePb:     a.Role,
					Role:       role,
					Who:        p,
					WhoHref:    whoHref,
					Prefix:     prefix,
					PrefixHref: prefixPageURL(m.Prefix),
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

	return out, nil
}

func fetchCallerRoles(c context.Context, pfx string) (*metadataBlock, error) {
	roles, err := impl.PublicRepo.GetRolesInPrefix(c, &api.PrefixRequest{
		Prefix: pfx,
	})
	if err != nil {
		return nil, err
	}
	out := &metadataBlock{
		CanView:     false,
		CallerRoles: make([]string, len(roles.Roles)),
	}
	for i, r := range roles.Roles {
		out.CallerRoles[i] = strings.Title(strings.ToLower(r.Role.String()))
	}
	return out, nil
}
