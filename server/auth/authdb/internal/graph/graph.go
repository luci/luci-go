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

// Package graph implements traversal of the groups graph.
//
// Such graphs are built from list of AuthGroup proto messages that reference
// each other by name.
package graph

import (
	"fmt"
	"math"

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
		return nil, fmt.Errorf("auth: too many groups (%d > %d)", len(groups), math.MaxUint16-1)
	}

	// Build all nodes, but don't connect them yet.
	graph := &Graph{
		Nodes:       make([]Node, len(groups)),
		NodesByName: make(map[string]NodeIndex, len(groups)),
	}
	for idx, g := range groups {
		if _, ok := graph.NodesByName[g.Name]; ok {
			return nil, fmt.Errorf("auth: bad AuthDB, group %q is listed twice", g.Name)
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
