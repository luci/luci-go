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
	// First pass: build all `group` nodes.
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
	// Second pass: fill in `nested` with pointers, now that we have them.
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
func (g *Groups) IsMember(id identity.Identity, groupName string) bool {
	// Cycle detection check uses a stack of groups currently being explored. Use
	// stack allocated array as a backing store to avoid unnecessary dynamic
	// allocation. If stack depth grows beyond 8, 'append' will reallocate it on
	// the heap.
	var backingStore [8]*group
	current := backingStore[:0]

	// Keep a set of all visited groups to avoid revisiting them in case of a
	// diamond-like graph, e.g A -> B, A -> C, B -> D, C -> D (we don't need to
	// visit D twice in this case).
	visited := make(map[*group]struct{}, 10)

	// isMember is used to recurse over nested groups.
	var isMember func(*group) bool

	isMember = func(gr *group) bool {
		// 'id' is a direct member?
		if _, ok := gr.members[id]; ok {
			return true
		}
		// 'id' matches some glob?
		for _, glob := range gr.globs {
			if glob.Match(id) {
				return true
			}
		}
		if len(gr.nested) == 0 {
			return false
		}
		current = append(current, gr) // popped before return
		found := false

	outer_loop:
		for _, nested := range gr.nested {
			// There should be no cycles, but do the check just in case there are,
			// seg faulting with stack overflow is very bad. In case of a cycle, skip
			// the offending group, but keep searching other groups.
			for _, ancestor := range current {
				if ancestor == nested {
					continue outer_loop
				}
			}
			// Explored 'nested' already (and didn't find anything) while visiting
			// some sibling branch? Skip.
			if _, seen := visited[nested]; seen {
				continue
			}
			if isMember(nested) {
				found = true
				break
			}
		}

		// Note that we don't use defers here since they have non-negligible runtime
		// cost. Using 'defer' here makes IsMember ~1.7x slower (1200 ns vs 700 ns),
		// See BenchmarkIsMember.
		current = current[:len(current)-1]
		visited[gr] = struct{}{}
		return found
	}

	if gr := g.groups[groupName]; gr != nil {
		return isMember(gr)
	}
	return false
}
