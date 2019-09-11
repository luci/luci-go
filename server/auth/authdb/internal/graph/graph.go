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

	"go.chromium.org/luci/server/auth/service/protocol"
)

// Graph is a static group graph optimized for traversals.
type Graph struct {
	Nodes map[string]*Node // group name => its node in the graph

	descendants map[*Node]NodeSet // node => all its children + node itself
	ancestors   map[*Node]NodeSet // node => all its ancestors + node itself
}

// NodeSet is a set of nodes.
type NodeSet map[*Node]struct{}

// Update adds all nodes in 'another' to 'ns'.
func (ns NodeSet) Update(another NodeSet) {
	for n := range another {
		ns[n] = struct{}{}
	}
}

// Node is a node in a group graph.
type Node struct {
	*protocol.AuthGroup // the original group proto

	Nested  []*Node // directly nested groups
	Parents []*Node // direct parent (nesting) groups
}

// Build constructs the graph from a list of AuthGroup messages.
func Build(groups []*protocol.AuthGroup) (*Graph, error) {
	// Build all nodes, but don't connect them yet.
	nodes := make(map[string]*Node, len(groups))
	for _, g := range groups {
		if nodes[g.Name] != nil {
			return nil, fmt.Errorf("auth: bad AuthDB, group %q is listed twice", g.Name)
		}
		nodes[g.Name] = &Node{
			AuthGroup: g,
			Nested:    make([]*Node, 0, len(g.Nested)),
		}
	}

	// Connect nodes by filling in Nested and Parents with pointers, now that we
	// have them all.
	for _, g := range groups {
		n := nodes[g.Name]
		for _, nested := range g.Nested {
			if nestedGr := nodes[nested]; nestedGr != nil {
				n.Nested = append(n.Nested, nestedGr)
				nestedGr.Parents = append(nestedGr.Parents, n)
			}
		}
	}

	return &Graph{
		Nodes:       nodes,
		descendants: make(map[*Node]NodeSet, len(nodes)),
		ancestors:   make(map[*Node]NodeSet, len(nodes)),
	}, nil
}

// Descendants returns a set with 'g' and all groups it includes.
func (g *Graph) Descendants(n *Node) NodeSet {
	if ns, ok := g.descendants[n]; ok {
		// Here we either have evaluated descendants of 'n' already (they are in
		// 'ns'), or we are recursively evaluating them right now (i.e. 'n' is
		// already being visited somewhere up the call stack). In the later case we
		// return an empty set to avoid infinite recursion.
		return ns
	}
	g.descendants[n] = nil // marker that we are traversing this subtree right now
	out := NodeSet{n: struct{}{}}
	for _, p := range n.Nested {
		out.Update(g.Descendants(p))
	}
	g.descendants[n] = out // the traversal is done
	return out
}

// Ancestors returns a set with 'g' and all groups that include it.
func (g *Graph) Ancestors(n *Node) NodeSet {
	if an, ok := g.ancestors[n]; ok {
		return an
	}
	g.ancestors[n] = nil
	out := NodeSet{n: struct{}{}}
	for _, p := range n.Parents {
		out.Update(g.Ancestors(p))
	}
	g.ancestors[n] = out
	return out
}
