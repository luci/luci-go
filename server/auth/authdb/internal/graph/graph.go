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

// Package graph implements handling of the groups graph.
//
// Such graphs are built from list of AuthGroup proto messages that reference
// each other by name.
package graph

import (
	"encoding/binary"
	"math"
	"sort"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/auth/authdb/internal/globset"
	"go.chromium.org/luci/server/auth/service/protocol"
)

// Graph is a static group graph optimized for traversals.
//
// Not safe for concurrent use.
type Graph struct {
	Nodes       []Node               // all graph nodes
	NodesByName map[string]NodeIndex // name => index in Nodes
}

// NodeIndex is an index of a node within graph's list of nodes.
//
// Used essentially as a pointer that occupies x4 less memory than the real one.
//
// Note: when changing the type, make sure to also change SortedNodeSet.MapKey
// and replace math.MaxUint16 in Build(...) with another bound.
type NodeIndex uint16

// NodeSet is a set of nodes referred by their indexes.
type NodeSet map[NodeIndex]struct{}

// Add adds node 'n' to 'ns'.
func (ns NodeSet) Add(n NodeIndex) {
	ns[n] = struct{}{}
}

// Update adds all nodes in 'another' to 'ns'.
func (ns NodeSet) Update(another NodeSet) {
	for n := range another {
		ns[n] = struct{}{}
	}
}

// Sort converts the NodeSet to SortedNodeSet.
func (ns NodeSet) Sort() SortedNodeSet {
	s := make(SortedNodeSet, 0, len(ns))
	for n := range ns {
		s = append(s, n)
	}
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s
}

// SortedNodeSet is a compact representation of NodeSet as a sorted slice.
type SortedNodeSet []NodeIndex

// Has is true if 'idx' is in 'ns'.
func (ns SortedNodeSet) Has(idx NodeIndex) bool {
	// TODO(vadimsh): Inline sort.Search? Inlining it (thus reducing overhead
	// on the callback call) makes it 3x faster in a synthetic test when searching
	// for a median in a slice with 50 items.
	found := sort.Search(len(ns), func(i int) bool { return ns[i] >= idx })
	return found < len(ns) && ns[found] == idx
}

// Intersects is true if 'a' and 'b' have common elements.
func (a SortedNodeSet) Intersects(b SortedNodeSet) bool {
	// Note: this is O(|a|+|b|) but with extremely small constant factor. It is
	// possible (and likely common) that |a| and |b| are significantly different.
	// In this case it may be more effective to use O(|a|*log|b|) (a binary search
	// in a loop), but its constant factor is larger due to overhead on function
	// calls (in particular inside the binary search implementation that uses
	// sort.Search). Synthetic benchmark shows O(50+5) algorithm is 10x faster
	// than O(5*log50) one. In practice both variants probably run in nanoseconds,
	// so it doesn't really matter.
	lenA, lenB := len(a), len(b)
	idxA, idxB := 0, 0
	for idxA < lenA && idxB < lenB {
		switch itemA, itemB := a[idxA], b[idxB]; {
		case itemA < itemB:
			idxA++
		case itemA > itemB:
			idxB++
		default: // equal
			return true
		}
	}
	return false
}

// MapKey converts 'ns' to a string that can be used as a map key.
func (ns SortedNodeSet) MapKey() string {
	buf := make([]byte, 2*len(ns))
	for i, x := range ns {
		binary.LittleEndian.PutUint16(buf[2*i:], uint16(x))
	}
	return string(buf)
}

// NodeSetDedupper helps to find duplicate NodeSet's.
type NodeSetDedupper map[string]SortedNodeSet

// Dedup returns a sorted version of 'ns' (perhaps reusing an existing one).
func (nsd NodeSetDedupper) Dedup(ns NodeSet) SortedNodeSet {
	sorted := ns.Sort()
	key := sorted.MapKey()
	if existing, ok := nsd[key]; ok {
		return existing
	}
	nsd[key] = sorted
	return sorted
}

// Node is a node in a group graph.
type Node struct {
	*protocol.AuthGroup // the original group proto

	Nested  []NodeIndex // directly nested groups
	Parents []NodeIndex // direct parent (nesting) groups
	Index   NodeIndex   // index of this node within the graph's list of nodes

	visitingNow bool    // true if traversed by Descendants/Ancestors right now
	descendants NodeSet // all children + self (computed lazily)
	ancestors   NodeSet // all ancestors + self (computed lazily)
}

// Build constructs the graph from a list of AuthGroup messages.
func Build(groups []*protocol.AuthGroup) (*Graph, error) {
	if len(groups) > math.MaxUint16-1 {
		return nil, errors.Reason("too many groups (%d > %d)", len(groups), math.MaxUint16-1).Err()
	}

	// Build all nodes, but don't connect them yet.
	graph := &Graph{
		Nodes:       make([]Node, len(groups)),
		NodesByName: make(map[string]NodeIndex, len(groups)),
	}
	for idx, g := range groups {
		if _, ok := graph.NodesByName[g.Name]; ok {
			return nil, errors.Reason("bad AuthDB, group %q is listed twice", g.Name).Err()
		}
		graph.NodesByName[g.Name] = NodeIndex(idx)
		graph.Nodes[idx] = Node{
			AuthGroup: g,
			Index:     NodeIndex(idx),
			Nested:    make([]NodeIndex, 0, len(g.Nested)),
		}
	}

	// Connect nodes by filling in Nested and Parents with indexes, now that we
	// have them all. All referenced nested groups must be present, but we ignore
	// unknown ones just in case.
	for idx := range graph.Nodes {
		n := &graph.Nodes[idx]
		for _, nested := range n.AuthGroup.Nested {
			if nestedGr := graph.NodeByName(nested); nestedGr != nil {
				n.Nested = append(n.Nested, nestedGr.Index)
				nestedGr.Parents = append(nestedGr.Parents, n.Index)
			}
		}
	}

	return graph, nil
}

// NodeByName returns a node given its name or nil if not found.
func (g *Graph) NodeByName(name string) *Node {
	if idx, ok := g.NodesByName[name]; ok {
		return &g.Nodes[idx]
	}
	return nil
}

// Visit passes each node in the set to the callback (in arbitrary order).
//
// Stops on a first error returning it as is. Panics if 'ns' has invalid
// indexes.
func (g *Graph) Visit(ns NodeSet, cb func(n *Node) error) error {
	for idx := range ns {
		if err := cb(&g.Nodes[idx]); err != nil {
			return err
		}
	}
	return nil
}

// Descendants returns a set with 'n' and all groups it includes.
func (g *Graph) Descendants(n NodeIndex) NodeSet {
	// Do not recurse into 'n' if we are traversing it already to avoid infinite
	// recursion.
	node := &g.Nodes[n]
	if node.visitingNow {
		return nil
	}
	if node.descendants != nil {
		return node.descendants // have been here before
	}
	node.visitingNow = true
	node.descendants = make(NodeSet, 1+len(node.Nested))
	node.descendants.Add(n)
	for _, x := range node.Nested {
		node.descendants.Update(g.Descendants(x))
	}
	node.visitingNow = false
	return node.descendants
}

// Ancestors returns a set with 'n' and all groups that include it.
func (g *Graph) Ancestors(n NodeIndex) NodeSet {
	// Do not recurse into 'n' if we are traversing it already to avoid infinite
	// recursion.
	node := &g.Nodes[n]
	if node.visitingNow {
		return nil
	}
	if node.ancestors != nil {
		return node.ancestors // have been here before
	}
	node.visitingNow = true
	node.ancestors = make(NodeSet, 1+len(node.Parents))
	node.ancestors.Add(n)
	for _, x := range node.Parents {
		node.ancestors.Update(g.Ancestors(x))
	}
	node.visitingNow = false
	return node.ancestors
}

// ToQueryable converts the graph to a form optimized for IsMember queries.
func (g *Graph) ToQueryable() (*QueryableGraph, error) {
	globs, err := g.buildGlobsMap()
	if err != nil {
		return nil, errors.Annotate(err, "failed to build globs map").Err()
	}
	return &QueryableGraph{
		groups:      g.NodesByName,
		memberships: g.buildMembershipsMap(),
		globs:       globs,
	}, nil
}

// buildGlobsMap builds a mapping: a group => all globs inside it (perhaps
// indirectly).
func (g *Graph) buildGlobsMap() (map[NodeIndex]globset.GlobSet, error) {
	builder := globset.NewBuilder()
	globs := make(map[NodeIndex]globset.GlobSet, 0)

	for idx := range g.Nodes {
		idx := NodeIndex(idx)

		// Reuse the builder to benefit from its cache of compiled regexps.
		builder.Reset()

		// Visit all descendants (all subgroups) of 'idx' to collect all globs
		// there.
		err := g.Visit(g.Descendants(idx), func(n *Node) error {
			for _, glob := range n.Globs {
				if err := builder.Add(identity.Glob(glob)); err != nil {
					return errors.Annotate(err, "bad glob %q in group %q", glob, n.Name).Err()
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		switch globSet, err := builder.Build(); {
		case err != nil:
			return nil, errors.Annotate(err, "bad glob pattern when traversing %q", g.Nodes[idx].Name).Err()
		case globSet != nil:
			globs[idx] = globSet
		}
	}

	return globs, nil
}

// buildMembershipsMap builds a map:
// normalized identity => groups it belongs to.
//
// Considers only direct mentions of identities in Members field of groups
// (i.e. ignores globs).
func (g *Graph) buildMembershipsMap() map[identity.NormalizedIdentity]SortedNodeSet {
	sets := make(map[identity.NormalizedIdentity]NodeSet) // normalized identity => groups it belongs to
	for idx, node := range g.Nodes {
		if len(node.Members) == 0 {
			continue
		}
		ancestors := g.Ancestors(NodeIndex(idx))
		for _, m := range node.Members {
			ident := identity.Identity(m).AsNormalized()
			nodeSet := sets[ident]
			if nodeSet == nil {
				nodeSet = make(NodeSet, len(ancestors))
				sets[ident] = nodeSet
			}
			nodeSet.Update(ancestors)
		}
	}

	// Convert sets to slices and find duplicates to reduce memory footprint.
	memberships := make(map[identity.NormalizedIdentity]SortedNodeSet, len(sets))
	dedupper := NodeSetDedupper{}
	for ident, nodeSet := range sets {
		memberships[ident] = dedupper.Dedup(nodeSet)
	}

	return memberships
}

// QueryableGraph is a processed Graph optimized for IsMember queries and low
// memory footprint.
//
// It is built from Graph via ToQueryable method. It is static once constructed
// and can be queried concurrently.
//
// TODO(vadimsh): Optimize 'memberships' to take less memory. It turns out
// string keys are quite expensive in terms of memory: a totally empty
// preallocated map[identity.Identity]SortedNodeSet (with empty keys!) is
// already *half* the size of the fully populated one.
type QueryableGraph struct {
	groups      map[string]NodeIndex                          // group name => group index
	memberships map[identity.NormalizedIdentity]SortedNodeSet // identity => groups it belongs to
	globs       map[NodeIndex]globset.GlobSet                 // group index => globs inside it
}

// BuildQueryable constructs the queryable graph from a list of AuthGroups.
func BuildQueryable(groups []*protocol.AuthGroup) (*QueryableGraph, error) {
	g, err := Build(groups)
	if err != nil {
		return nil, err
	}
	return g.ToQueryable()
}

// GroupIndex returns a NodeIndex of the group given its name.
func (g *QueryableGraph) GroupIndex(group string) (idx NodeIndex, ok bool) {
	idx, ok = g.groups[group]
	return
}

// IsMemberResult is the possible results for a 'ident in group' check
// implemented by QueryableGraph.IsMember.
type IsMemberResult byte

const (
	IdentIsNotMember IsMemberResult = iota
	IdentIsMember
	GroupIsUnknown
)

// IsMember returns true if the given identity belongs to the given group.
func (g *QueryableGraph) IsMember(ident identity.Identity, group string) IsMemberResult {
	if grpIdx, ok := g.groups[group]; ok {
		if g.memberships[ident.AsNormalized()].Has(grpIdx) || g.globs[grpIdx].Has(ident) {
			return IdentIsMember
		}
		return IdentIsNotMember
	}
	return GroupIsUnknown
}

// MembershipsQueryCache prepares a query for memberships of the given identity.
//
// This query can be used to answer a bunch of IsMemberOfAny questions, caching
// some internal state in-between them.
func (g *QueryableGraph) MembershipsQueryCache(ident identity.Identity) MembershipsQueryCache {
	normIdent := ident.AsNormalized()
	return MembershipsQueryCache{
		Identity:           ident,
		NormalizedIdentity: normIdent,
		graph:              g,
		memberships:        g.memberships[normIdent],
	}
}

// MembershipsQueryCache can be used to answer a bunch of IsMemberOfAny
// questions, caching some internal state in-between them.
type MembershipsQueryCache struct {
	// Identity whose memberships are being queried.
	Identity identity.Identity
	// The normalized version of Identity.
	NormalizedIdentity identity.NormalizedIdentity

	graph       *QueryableGraph
	memberships SortedNodeSet
}

// IsMemberOfAny returns true if the identity belongs to any of the given
// groups.
//
// Groups are given as a sorted slice of group indexes obtained via GroupIndex.
func (q *MembershipsQueryCache) IsMemberOfAny(groups SortedNodeSet) bool {
	if q.memberships.Intersects(groups) {
		return true
	}
	// The above check works only for identities mentioned in the group graph
	// directly. We still need to check whether `q.Identity` belongs to any of the
	// groups through a glob.
	for _, grpIdx := range groups {
		if q.graph.globs[grpIdx].Has(q.Identity) {
			return true
		}
	}
	return false
}
