// Copyright 2019 The LUCI Authors.
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

// Package legacy contains older implementation of IsMember check.
//
// To be deleted very soon.
package legacy

import (
	"fmt"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/server/auth/authdb/internal/graph"
	"go.chromium.org/luci/server/auth/service/protocol"
)

// Groups is a legacy representation of the groups graph.
type Groups struct {
	groups map[string]*group // map of all known groups
}

// group is a node in a group graph. Nested groups are referenced directly via
// pointer.
type group struct {
	members map[identity.Identity]struct{} // set of all members
	globs   []identity.Glob                // list of all identity globs
	nested  []*group                       // pointers to nested groups
}

// BuildGroups builds the legacy representation of the groups graph.
func BuildGroups(groups []*protocol.AuthGroup) (*Groups, error) {
	grs := make(map[string]*group, len(groups))
	for _, g := range groups {
		if grs[g.Name] != nil {
			return nil, fmt.Errorf("auth: bad AuthDB, group %q is listed twice", g.Name)
		}
		gr := &group{}
		if len(g.Members) != 0 {
			gr.members = make(map[identity.Identity]struct{}, len(g.Members))
			for _, ident := range g.Members {
				gr.members[identity.Identity(ident)] = struct{}{}
			}
		}
		if len(g.Globs) != 0 {
			gr.globs = make([]identity.Glob, len(g.Globs))
			for i, glob := range g.Globs {
				gr.globs[i] = identity.Glob(glob)
			}
		}
		if len(g.Nested) != 0 {
			gr.nested = make([]*group, 0, len(g.Nested))
		}
		grs[g.Name] = gr
	}

	for _, g := range groups {
		gr := grs[g.Name]
		for _, nestedName := range g.Nested {
			if nestedGroup := grs[nestedName]; nestedGroup != nil {
				gr.nested = append(gr.nested, nestedGroup)
			}
		}
	}

	return &Groups{grs}, nil
}

// IsMember returns true if the given identity belongs to the given group.
func (g *Groups) IsMember(id identity.Identity, groupName string) graph.IsMemberResult {
	var backingStore [8]*group
	current := backingStore[:0]

	visited := make(map[*group]struct{}, 10)

	var isMember func(*group) bool

	isMember = func(gr *group) bool {
		if _, ok := gr.members[id]; ok {
			return true
		}
		for _, glob := range gr.globs {
			if glob.Match(id) {
				return true
			}
		}
		if len(gr.nested) == 0 {
			return false
		}
		current = append(current, gr)
		found := false

	outer_loop:
		for _, nested := range gr.nested {
			for _, ancestor := range current {
				if ancestor == nested {
					continue outer_loop
				}
			}
			if _, seen := visited[nested]; seen {
				continue
			}
			if isMember(nested) {
				found = true
				break
			}
		}

		current = current[:len(current)-1]
		visited[gr] = struct{}{}
		return found
	}

	if gr := g.groups[groupName]; gr != nil {
		if isMember(gr) {
			return graph.IdentIsMember
		}
		return graph.IdentIsNotMember
	}
	return graph.GroupIsUnknown
}
