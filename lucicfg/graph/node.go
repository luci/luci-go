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

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"go.chromium.org/luci/starlark/builtins"
)

// Node is an element of the graph.
type Node struct {
	Key   *Key                         // unique ID of the node
	Props *starlarkstruct.Struct       // struct(...) with frozen properties
	Trace *builtins.CapturedStacktrace // where the node was defined

	children []*Edge // edges from us (the parent) to the children
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
func (n *Node) declare(props *starlarkstruct.Struct, trace *builtins.CapturedStacktrace) {
	props.Freeze()
	n.Props = props
	n.Trace = trace
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

// String is a part of starlark.Value interface.
//
// Returns a node title as derived from the last component of its key. It's not
// globally unique, but usually "unique enough" to identify the node in error
// messages.
func (n *Node) String() string {
	kind, id := n.Key.Last()
	return fmt.Sprintf("%s(%q)", kind, id)
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
