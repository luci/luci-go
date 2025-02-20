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

var (
	// ErrNoSuchGroup is returned when a group is not found in the groups graph.
	ErrNoSuchGroup = errors.New("no such group")

	// ErrInvalidPrincipalKind is returned when a principal has an invalid kind.
	ErrInvalidPrincipalKind = errors.New("invalid principal kind")

	// ErrInvalidPrincipalValue is returned when a principal has an invalid value.
	ErrInvalidPrincipalValue = errors.New("invalid principal value")
)

// Graph represents a traversable group graph.
type Graph struct {
	// All graph nodes, key is group name.
	groups map[string]*GroupNode
	// All known globs sorted alphabetically.
	globs []identity.Glob
	// Group names that directly include the given identity.
	membersIndex map[identity.NormalizedIdentity][]string
	// Group names that directly include the given glob.
	globsIndex map[identity.Glob][]string
}

// GroupNode contains information related to an individual group.
type GroupNode struct {
	group model.GraphableGroup

	includes []*GroupNode // groups directly included by this group.
	included []*GroupNode // groups that directly include this group.
}

// initializeNodes initializes the groupNode(s) in the graph
// it creates a groupNode for every group in the datastore.
func (g *Graph) initializeNodes(groups []model.GraphableGroup) {
	for _, group := range groups {
		name := group.GetName()
		g.groups[name] = &GroupNode{group: group}
		// Populate globsIndex.
		for _, glob := range group.GetGlobs() {
			identityGlob := identity.Glob(glob)
			g.globsIndex[identityGlob] = append(g.globsIndex[identityGlob], name)
		}

		// Populate members.
		for _, member := range group.GetMembers() {
			memberIdentity := identity.NewNormalizedIdentity(member)
			g.membersIndex[memberIdentity] = append(g.membersIndex[memberIdentity], name)
		}
	}

	// Populate includes and included.
	for _, parent := range groups {
		for _, nestedID := range parent.GetNested() {
			if nested, ok := g.groups[nestedID]; ok {
				parentName := parent.GetName()
				g.groups[parentName].includes = append(g.groups[parentName].includes, nested)
				nested.included = append(nested.included, g.groups[parentName])
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
func NewGraph(groups []model.GraphableGroup) *Graph {
	graph := &Graph{
		groups:       make(map[string]*GroupNode, len(groups)),
		membersIndex: map[identity.NormalizedIdentity][]string{},
		globsIndex:   map[identity.Glob][]string{},
	}

	graph.initializeNodes(groups)

	return graph
}

// ExpandedGroup can represent a fully expanded AuthGroup, with all
// memberships listed from both direct and indirect inclusions.
type ExpandedGroup struct {
	Name     string
	Members  stringset.Set
	Redacted stringset.Set
	Globs    stringset.Set
	Nested   stringset.Set
}

// Absorb updates this ExpandedGroup's memberships to include the memberships
// in the other ExpandedGroup.
func (e *ExpandedGroup) Absorb(other *ExpandedGroup) {
	e.Members.AddAll(other.Members.ToSlice())
	e.Redacted.AddAll(other.Redacted.ToSlice())
	e.Globs.AddAll(other.Globs.ToSlice())
	e.Nested.AddAll(other.Nested.ToSlice())
}

// ToProto converts an ExpandedGroup to a rpcpb.AuthGroup.
func (e *ExpandedGroup) ToProto() *rpcpb.AuthGroup {
	// Remove known members from redacted; they are part of the group some other
	// way.
	redacted := e.Redacted.Difference(e.Members)

	return &rpcpb.AuthGroup{
		Name:        e.Name,
		Members:     e.Members.ToSortedSlice(),
		Globs:       e.Globs.ToSortedSlice(),
		Nested:      e.Nested.ToSortedSlice(),
		NumRedacted: int32(len(redacted)),
	}
}

// ExpansionCache is a map of groups which have already been expanded.
type ExpansionCache struct {
	Groups map[string]*ExpandedGroup
}

func (g *Graph) doExpansion(
	ctx context.Context,
	node *GroupNode,
	skipFilter bool,
	cache *ExpansionCache) (*ExpandedGroup, error) {
	name := node.group.GetName()

	// Check the cache first.
	if cachedResult, ok := cache.Groups[name]; ok {
		return cachedResult, nil
	}

	// Initialize this group's memberships. Direct globs and nested subgroups can
	// be included regardless of the filter.
	nested := node.group.GetNested()
	result := &ExpandedGroup{
		Name:     name,
		Members:  stringset.New(0),
		Redacted: stringset.New(0),
		Globs:    stringset.NewFromSlice(node.group.GetGlobs()...),
		Nested:   stringset.NewFromSlice(nested...),
	}

	// Add direct members depending on the filter.
	members := node.group.GetMembers()
	if skipFilter {
		result.Members.AddAll(members)
	} else {
		// Check whether the caller can view members.
		ok, err := model.CanCallerViewMembers(ctx, node.group)
		if err != nil {
			return nil, err
		}
		if ok {
			result.Members.AddAll(members)
		} else {
			result.Redacted.AddAll(members)
		}
	}

	// Process the memberships from nested groups.
	for _, subgroup := range node.includes {
		subName := subgroup.group.GetName()

		var expandedSub *ExpandedGroup
		// Check the cache for this subgroup.
		if cachedSub, ok := cache.Groups[subName]; ok {
			expandedSub = cachedSub
		}
		if expandedSub == nil {
			// Expand the subgroup.
			var err error
			expandedSub, err = g.doExpansion(ctx, subgroup, skipFilter, cache)
			if err != nil {
				return nil, err
			}
		}

		// Add the subgroup's membership's to this group's memberships.
		result.Absorb(expandedSub)
	}

	// Update the cache for this group.
	cache.Groups[name] = result

	return result, nil
}

// GetExpandedGroup returns the explicit membership rules for the group.
//
// Note: a privacy filter for members was added in Auth Service v2. To support
// legacy endpoints and maintain the existing behavior of Auth Service v1,
// the privacy filter can be disabled with `skipFilter` set to `true`.
//
// If the group exists in the Graph, the returned ExpandedGroup shall have the
// following fields:
//   - Name, the name of the group;
//   - Members, containing all unique members from both direct and indirect
//     inclusions;
//   - Globs, containing all unique globs from both direct and indirect
//     inclusions; and
//   - Nested, containing all unique nested groups from both direct and indirect
//     inclusions.
//   - Redacted, containing all unique members which were redacted from both
//     direct and indirect inclusions.
func (g *Graph) GetExpandedGroup(
	ctx context.Context, name string, skipFilter bool,
	cache *ExpansionCache) (*ExpandedGroup, error) {
	root, ok := g.groups[name]
	if !ok {
		return nil, ErrNoSuchGroup
	}

	if cache == nil {
		cache = &ExpansionCache{
			Groups: make(map[string]*ExpandedGroup),
		}
	}

	return g.doExpansion(ctx, root, skipFilter, cache)
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
		for _, group := range g.membersIndex[ident.AsNormalized()] {
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
			s.addEdge(groupID, g.traverse(supergroup.group.GetName(), s))
		}
	}
	return groupID
}

// ConvertPrincipal handles the conversion of rpcpb.Principal -> graph.NodeKey.
func ConvertPrincipal(p *rpcpb.Principal) (NodeKey, error) {
	if p.Name == "" {
		return NodeKey{}, ErrInvalidPrincipalValue
	}

	switch p.Kind {
	case rpcpb.PrincipalKind_GLOB:
		return NodeKey{Kind: Glob, Value: p.Name}, nil
	case rpcpb.PrincipalKind_IDENTITY:
		return NodeKey{Kind: Identity, Value: p.Name}, nil
	case rpcpb.PrincipalKind_GROUP:
		return NodeKey{Kind: Group, Value: p.Name}, nil
	default:
		return NodeKey{}, ErrInvalidPrincipalKind
	}
}
