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

package graph

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"go.chromium.org/luci/starlark/builtins"
)

// Node is an element of the graph.
type Node struct {
	Key        *Key                         // unique ID of the node
	Index      int                          // index of the node in the graph
	Props      *starlarkstruct.Struct       // struct(...) with frozen properties
	Trace      *builtins.CapturedStacktrace // where the node was defined
	Idempotent bool                         // true if it's fine to redeclare this node

	children []*Edge // edges from us (the parent) to the children
	parents  []*Edge // edges from the parents to us (the child)

	childrenList []*Node // memoized result of listChildren()
	parentsList  []*Node // memoised result of listParents()
}

// Edge is a single 'Parent -> Child' edge in the graph.
type Edge struct {
	Parent *Node
	Child  *Node
	Title  string                       // arbitrary title for error messages
	Trace  *builtins.CapturedStacktrace // where it was defined
}

// declare marks the node as declared.
//
// Freezes properties as a side effect.
func (n *Node) declare(idx int, props *starlarkstruct.Struct, idempotent bool, trace *builtins.CapturedStacktrace) {
	props.Freeze()
	n.Index = idx
	n.Props = props
	n.Idempotent = idempotent
	n.Trace = trace
}

// BelongsTo returns true if the node was declared in the given graph.
func (n *Node) BelongsTo(g *Graph) bool {
	return n.Key.set == &g.KeySet
}

// Declared is true if the node was fully defined via AddNode and false if
// it was only "predeclared" by being referenced in some edge (via AddEdge).
//
// In a finalized graph there are no dangling edges: all nodes are guaranteed to
// be declared.
func (n *Node) Declared() bool {
	return n.Props != nil
}

// visitDescendants calls cb(n, path) and then recursively visits all
// descendants of 'n' in depth first order until a first error.
//
// If a descendant is reachable through multiple paths, it is visited multiple
// times.
//
// 'path' contains a path being explored now, defined as a list of visited
// edges. The array backing this slice is mutated by 'visitDescendants', so if
// 'cb' wants to preserve it, it needs to make a copy.
//
// Assumes the graph has no cycles (as verified by AddEdge).
func (n *Node) visitDescendants(path []*Edge, cb func(*Node, []*Edge) error) error {
	if err := cb(n, path); err != nil {
		return err
	}
	for _, e := range n.children {
		if err := e.Child.visitDescendants(append(path, e), cb); err != nil {
			return err
		}
	}
	return nil
}

// listChildren returns a slice with a set of direct children of the node, in
// order corresponding edges were declared.
//
// Must be used only with finalized graphs, since the function caches its result
// internally on a first call. Returns a copy of the cached result.
func (n *Node) listChildren() []*Node {
	if n.childrenList == nil {
		// Note: we want to allocate a new slice even if it is empty, so that
		// n.childrenList is not nil anymore (to indicate we did the calculation
		// already).
		n.childrenList = make([]*Node, 0, len(n.children))
		seen := make(map[*Key]struct{}, len(n.children))
		for _, edge := range n.children {
			if _, ok := seen[edge.Child.Key]; !ok {
				seen[edge.Child.Key] = struct{}{}
				n.childrenList = append(n.childrenList, edge.Child)
			}
		}
	}
	return append([]*Node(nil), n.childrenList...)
}

// listParents returns a slice with a set of direct parents of the node, in
// order corresponding edges were declared.
//
// Must be used only with finalized graphs, since the function caches its result
// internally on a first call. Returns a copy of the cached result.
func (n *Node) listParents() []*Node {
	if n.parentsList == nil {
		// Note: we want to allocate a new slice even if it is empty, so that
		// n.parentsList is not nil anymore (to indicate we did the calculation
		// already).
		n.parentsList = make([]*Node, 0, len(n.parents))
		seen := make(map[*Key]struct{}, len(n.parents))
		for _, edge := range n.parents {
			if _, ok := seen[edge.Parent.Key]; !ok {
				seen[edge.Parent.Key] = struct{}{}
				n.parentsList = append(n.parentsList, edge.Parent)
			}
		}
	}
	return append([]*Node(nil), n.parentsList...)
}

// String is a part of starlark.Value interface.
//
// Returns a node title as derived from the kind of last component of its key
// and IDs of all key components whose kind doesn't start with '_'. It's not
// 1-to-1 mapping to the full information in the key, but usually good enough to
// identify the node in error messages.
func (n *Node) String() string {
	ids := make([]string, 0, 5) // overestimate
	p := n.Key
	for p != nil {
		if !strings.HasPrefix(p.Kind(), "_") {
			ids = append(ids, p.ID())
		}
		p = p.Container()
	}
	for l, r := 0, len(ids)-1; l < r; l, r = l+1, r-1 {
		ids[l], ids[r] = ids[r], ids[l]
	}
	return fmt.Sprintf("%s(%q)", n.Key.Kind(), strings.Join(ids, "/"))
}

// Type is a part of starlark.Value interface.
func (n *Node) Type() string { return "graph.node" }

// Freeze is a part of starlark.Value interface.
func (n *Node) Freeze() {}

// Truth is a part of starlark.Value interface.
func (n *Node) Truth() starlark.Bool { return starlark.True }

// Hash is a part of starlark.Value interface.
func (n *Node) Hash() (uint32, error) { return n.Key.Hash() }

// AttrNames is a part of starlark.HasAttrs interface.
func (n *Node) AttrNames() []string {
	return []string{"key", "props", "trace"}
}

// Attr is a part of starlark.HasAttrs interface.
func (n *Node) Attr(name string) (starlark.Value, error) {
	switch name {
	case "key":
		return n.Key, nil
	case "props":
		return n.Props, nil
	case "trace":
		return n.Trace, nil
	default:
		return nil, nil
	}
}
