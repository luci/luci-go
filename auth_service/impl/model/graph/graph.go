// Copyright 2021 The LUCI Authors.
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

// Package graph contains groups graph definitions and operations.
package graph

import (
	"context"
	"errors"
	"sort"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
)

// ErrNoSuchGroup is returned when a group is not found in the groups graph.
var ErrNoSuchGroup = errors.New("no such group")

// Graph represents a traversable group graph.
type Graph struct {
	// All graph nodes, key is group name.
	groups map[string]*groupNode
	// All known globs sorted alphabetically.
	globs []identity.Glob
	// Group names that directly include the given identity.
	membersIndex map[identity.Identity][]string
	// Group names that directly include the given glob.
	globsIndex map[identity.Glob][]string
}

// groupNode contains information related to an individual group.
type groupNode struct {
	group *model.AuthGroup

	includes []*groupNode // groups directly included by this group.
	included []*groupNode // groups that directly include this group.
}

// initializeNodes initializes the groupNode(s) in the graph
// it creates a groupNode for every group in the datastore.
func (g *Graph) initializeNodes(groups []*model.AuthGroup) {
	for _, group := range groups {
		g.groups[group.ID] = &groupNode{group: group}
		// Populate globsIndex.
		for _, glob := range group.Globs {
			identityGlob := identity.Glob(glob)
			g.globsIndex[identityGlob] = append(g.globsIndex[identityGlob], group.ID)
		}

		// Populate members.
		for _, member := range group.Members {
			memberIdentity := identity.Identity(member)
			g.membersIndex[memberIdentity] = append(g.membersIndex[memberIdentity], group.ID)
		}
	}

	// Populate includes and included.
	for _, parent := range groups {
		for _, nestedID := range parent.Nested {
			if nested, ok := g.groups[nestedID]; ok {
				g.groups[parent.ID].includes = append(g.groups[parent.ID].includes, nested)
				nested.included = append(nested.included, g.groups[parent.ID])
			}
		}
	}

	// Sort globsIndex keys alphabetically to populate globs.
	g.globs = make([]identity.Glob, 0, len(g.globsIndex))
	for glob := range g.globsIndex {
		g.globs = append(g.globs, glob)
	}
	sort.Slice(g.globs, func(i, j int) bool {
		return g.globs[i] < g.globs[j]
	})
}

////////////////////////////////////////////////////////////////////////////////////////

// NewGraph creates all groupNode(s) that are available in the graph.
func NewGraph(groups []*model.AuthGroup) *Graph {
	graph := &Graph{
		groups:       make(map[string]*groupNode, len(groups)),
		membersIndex: map[identity.Identity][]string{},
		globsIndex:   map[identity.Glob][]string{},
	}

	graph.initializeNodes(groups)

	return graph
}

// GetExpandedGroup returns the explicit membership rules for the group.
//
// If the group exists in the Graph, the returned AuthGroup shall have the
// following fields:
//   - Name, the name of the group;
//   - Members, containing all unique members from both direct and indirect inclusions;
//   - Globs, containing all unique globs from both direct and indirect inclusions; and
//   - Nested, containing all unique nested groups from both direct and indirect
//     inclusions.
func (g *Graph) GetExpandedGroup(ctx context.Context, name string) (*rpcpb.AuthGroup, error) {
	root, ok := g.groups[name]
	if !ok {
		return nil, ErrNoSuchGroup
	}

	// The direct and indirect memberships of the fully expanded group.
	members := stringset.Set{}
	redactedMembers := stringset.Set{}
	globs := stringset.Set{}
	nested := stringset.Set{}

	// The set of groups which have already been expanded.
	expanded := stringset.Set{}

	var expand func(node *groupNode) error
	expand = func(node *groupNode) error {
		if expanded.Has(node.group.ID) {
			// Skip previously processed group.
			return nil
		}

		// Process the group's members, globs and nested groups.

		// Check whether the caller can view members.
		ok, err := model.CanCallerViewMembers(ctx, node.group)
		if err != nil {
			return err
		}
		if ok {
			members.AddAll(node.group.Members)
		} else {
			redactedMembers.AddAll(node.group.Members)
		}

		// Record the group's globs and nested subgroups.
		globs.AddAll(node.group.Globs)
		nested.AddAll(node.group.Nested)

		// Record the group as having been processed.
		expanded.Add(node.group.ID)

		// Process the memberships from subgroups of this group.
		for _, subgroup := range node.includes {
			if err := expand(subgroup); err != nil {
				return err
			}
		}

		return nil
	}

	// Expand memberships, starting from the given group.
	if err := expand(root); err != nil {
		return nil, err
	}

	// Remove known members from redacted; they are part of the group some other
	// way.
	knownMembers := members.ToSortedSlice()
	redactedMembers.DelAll(knownMembers)

	return &rpcpb.AuthGroup{
		Name:        root.group.ID,
		Members:     knownMembers,
		Globs:       globs.ToSortedSlice(),
		Nested:      nested.ToSortedSlice(),
		NumRedacted: int32(len(redactedMembers)),
	}, nil
}

// GetRelevantSubgraph returns a Subgraph of groups that
// include the principal.
//
// Subgraph is represented as series of nodes connected by labeled edges
// representing inclusion.
func (g *Graph) GetRelevantSubgraph(principal NodeKey) (*Subgraph, error) {
	subgraph := &Subgraph{
		nodesToID: map[NodeKey]int32{},
	}

	// Find the leaves of the graph. It's the only part that depends on the
	// exact kind of principal. Once we get to the leaf groups, everything is
	// uniform. After that, we just travel through the graph via traverse.
	switch principal.Kind {
	case Identity:
		rootID, _ := subgraph.addNode(principal.Kind, principal.Value)
		ident := identity.Identity(principal.Value)

		// Add globs that match identity and connect glob nodes to root.
		for _, glob := range g.globs {
			// Find all globs that match the identity. The identity will
			// belong to all the groups that the glob belongs to.
			if glob.Match(ident) {
				globID, _ := subgraph.addNode(Glob, string(glob))
				subgraph.addEdge(rootID, globID)
				for _, group := range g.globsIndex[glob] {
					subgraph.addEdge(globID, g.traverse(group, subgraph))
				}
			}
		}

		// Find all the groups that directly mention the identity.
		for _, group := range g.membersIndex[identity.Identity(principal.Value)] {
			subgraph.addEdge(rootID, g.traverse(group, subgraph))
		}
	case Glob:
		rootID, _ := subgraph.addNode(principal.Kind, principal.Value)

		// Find all groups that directly mention the glob.
		for _, group := range g.globsIndex[identity.Glob(principal.Value)] {
			subgraph.addEdge(rootID, g.traverse(group, subgraph))
		}
	case Group:
		// Return an error if principal value is non existant in groups graph.
		if _, ok := g.groups[principal.Value]; !ok {
			return nil, ErrNoSuchGroup
		}
		g.traverse(principal.Value, subgraph)
	default:
		return nil, errors.New("principal kind unknown")
	}

	return subgraph, nil
}

// traverse adds the given group and all groups that include it to the subgraph s.
// Traverses the group graph g from leaves (most nested groups) to
// roots (least nested groups). Returns the node id of the last visited node.
func (g *Graph) traverse(group string, s *Subgraph) int32 {
	groupID, added := s.addNode(Group, group)
	if added {
		groupNode := g.groups[group]
		for _, supergroup := range groupNode.included {
			s.addEdge(groupID, g.traverse(supergroup.group.ID, s))
		}
	}
	return groupID
}
