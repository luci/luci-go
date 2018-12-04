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
	"sort"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"go.chromium.org/luci/starlark/builtins"
)

// NodeRedeclarationError is returned when a adding an existing node.
type NodeRedeclarationError struct {
	Trace    *builtins.CapturedStacktrace // where it is added second time
	Previous *Node                        // the previously added node
}

// Error is part of 'error' interface.
func (e *NodeRedeclarationError) Error() string {
	// TODO(vadimsh): Improve error messages.
	return fmt.Sprintf("%s is redeclared, previous declaration:\n%s",
		e.Previous.Key, e.Previous.Trace)
}

// EdgeRedeclarationError is returned when adding an edge that has exact same
// end points and a title as some existing edge.
//
// Edges that link same nodes (in same direction) and differ only in title are
// OK and do not cause this error.
//
// Nodes referenced by the edge may not be fully declared yet (i.e. they may not
// yet have properties or trace associated with them). They do have valid keys
// though.
type EdgeRedeclarationError struct {
	Trace    *builtins.CapturedStacktrace // where it is added second time
	Previous *Edge                        // the previously added edge
}

// Error is part of 'error' interface.
func (e *EdgeRedeclarationError) Error() string {
	// TODO(vadimsh): Improve error messages.
	return fmt.Sprintf("Relation %q between %s and %s is redeclared, previous declaration:\n%s",
		e.Previous.Title, e.Previous.Parent.Key, e.Previous.Child.Key, e.Previous.Trace)
}

// CycleError is returned when adding an edge that introduces a cycle.
//
// Nodes referenced by edges may not be fully declared yet (i.e. they may not
// yet have properties or trace associated with them). They do have valid keys
// though.
type CycleError struct {
	Trace *builtins.CapturedStacktrace // where the edge is being added
	Edge  *Edge                        // an edge being added
	Path  []*Edge                      // the rest of the cycle
}

// Error is part of 'error' interface.
func (e *CycleError) Error() string {
	// TODO(vadimsh): Improve error messages.
	return fmt.Sprintf("Relation %q between %s and %s introduces a cycle",
		e.Edge.Title, e.Edge.Parent.Key, e.Edge.Child.Key)
}

// Graph is a DAG of keyed nodes.
//
// It implements starlark.HasAttrs interface and have the following methods:
//   * key(kind1: string, id1: string, kind2: string, id2: string, ...): graph.key
//   * add_node(key: graph.key, props={}, trace=stacktrace()): graph.node
//   * node(key: graph.key): graph.node
//   * add_edge(parent: graph.Key, child: graph.Key, title='', trace=stacktrace())
//
// TODO(vadimsh): Forbid querying the graph while it is being under
// construction (i.e. not frozen yet). Rules that add nodes must not depend
// on order in which they are called.
//
// TODO(vadimsh): Check the graph is fully defined when it is being frozen.
type Graph struct {
	KeySet

	nodes  map[*Key]*Node
	frozen bool
}

// validateKey returns an error if the key is not from this graph.
func (g *Graph) validateKey(title string, k *Key) error {
	if k.set != &g.KeySet {
		return fmt.Errorf("bad %s: %s is from another graph", title, k)
	}
	return nil
}

// initNode either returns an existing *Node, or adds a new one.
//
// The newly added node is in "predeclared" state: it has no Props or Trace.
// A predeclared node is moved to the fully declared state by AddNode, this can
// happen only once.
//
// Panics if the graph if frozen.
func (g *Graph) initNode(k *Key) *Node {
	if g.frozen {
		panic("the graph is frozen")
	}
	if n := g.nodes[k]; n != nil {
		return n
	}
	if g.nodes == nil {
		g.nodes = make(map[*Key]*Node, 1)
	}
	n := &Node{Key: k}
	g.nodes[k] = n
	return n
}

// AddNode adds a node to the graph or returns an error if such node already
// exists.
//
// Freezes props.values() as a side effect.
func (g *Graph) AddNode(k *Key, props *starlark.Dict, trace *builtins.CapturedStacktrace) (*Node, error) {
	if g.frozen {
		return nil, fmt.Errorf("cannot add a node to a frozen graph")
	}
	if err := g.validateKey("key", k); err != nil {
		return nil, err
	}

	// Only string keys are allowed in 'props'.
	for _, pk := range props.Keys() {
		if _, ok := pk.(starlark.String); !ok {
			return nil, fmt.Errorf("non-string key %s in 'props'", pk)
		}
	}

	// A node can be fully declared only once.
	n := g.initNode(k)
	if n.declared() {
		return nil, &NodeRedeclarationError{Trace: trace, Previous: n}
	}
	n.declare(starlarkstruct.FromKeywords(starlark.String("props"), props.Items()), trace)
	return n, nil
}

// Node returns a node by the key or nil if it wasn't added by AddNode yet.
func (g *Graph) Node(k *Key) *Node {
	if n := g.nodes[k]; n != nil && n.declared() {
		return n
	}
	return nil
}

// AddEdge adds an edge to the graph.
//
// Neither of the nodes have to exist yet: it is OK to declare nodes and edges
// in arbitrary order as long as at the end of the graph construction (when it
// is frozen) the graph is complete.
//
// Returns an error if there's already an edge between the given nodes with
// exact same title or the new edge introduces a cycle.
func (g *Graph) AddEdge(parent, child *Key, title string, trace *builtins.CapturedStacktrace) error {
	if g.frozen {
		return fmt.Errorf("cannot add an edge to a frozen graph")
	}
	if err := g.validateKey("parent", parent); err != nil {
		return err
	}
	if err := g.validateKey("child", child); err != nil {
		return err
	}

	edge := &Edge{
		Parent: g.initNode(parent),
		Child:  g.initNode(child),
		Title:  title,
		Trace:  trace,
	}

	// Have this exact edge already?
	for _, e := range edge.Parent.children {
		if e.Child == edge.Child && e.Title == title {
			return &EdgeRedeclarationError{
				Trace:    trace,
				Previous: e,
			}
		}
	}

	// The child may have the parent among its descendants already? Then adding
	// the edge would introduce a cycle.
	err := edge.Child.visitDescendants(nil, func(n *Node, path []*Edge) error {
		if n == edge.Parent {
			return &CycleError{
				Trace: trace,
				Edge:  edge,
				Path:  append([]*Edge(nil), path...),
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	edge.Parent.children = append(edge.Parent.children, edge)
	return nil
}

// String is a part of starlark.Value interface
func (g *Graph) String() string { return "graph" }

// Type is a part of starlark.Value interface.
func (g *Graph) Type() string { return "graph" }

// Freeze is a part of starlark.Value interface.
func (g *Graph) Freeze() { g.frozen = true }

// Truth is a part of starlark.Value interface.
func (g *Graph) Truth() starlark.Bool { return starlark.True }

// Hash is a part of starlark.Value interface.
func (g *Graph) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable") }

// AttrNames is a part of starlark.HasAttrs interface.
func (g *Graph) AttrNames() []string {
	names := make([]string, 0, len(graphAttrs))
	for k := range graphAttrs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Attr is a part of starlark.HasAttrs interface.
func (g *Graph) Attr(name string) (starlark.Value, error) {
	impl, ok := graphAttrs[name]
	if !ok {
		return nil, nil // per Attr(...) contract
	}
	return impl.BindReceiver(g), nil
}

//// Starlark bindings for individual graph methods.

var graphAttrs = map[string]*starlark.Builtin{
	// key(kind1: string, id1: string, kind2: string, id2: string, ...): graph.key
	"key": starlark.NewBuiltin("key", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if len(kwargs) != 0 {
			return nil, fmt.Errorf("graph.key: not expecting keyword arguments")
		}
		pairs := make([]string, len(args))
		for idx, arg := range args {
			str, ok := arg.(starlark.String)
			if !ok {
				return nil, fmt.Errorf("graph.key: all arguments must be strings, arg #%d was %s", idx, arg.Type())
			}
			pairs[idx] = str.GoString()
		}
		return b.Receiver().(*Graph).Key(pairs...)
	}),

	// add_node(key: graph.key, props={}, trace=stacktrace()): graph.node
	"add_node": starlark.NewBuiltin("add_node", func(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var key *Key
		var props *starlark.Dict
		var trace *builtins.CapturedStacktrace
		if err := starlark.UnpackArgs("add_node", args, kwargs, "key", &key, "props?", &props, "trace?", &trace); err != nil {
			return nil, err
		}
		if props == nil {
			props = &starlark.Dict{}
		}
		if trace == nil {
			var err error
			if trace, err = builtins.CaptureStacktrace(th, 0); err != nil {
				return nil, err
			}
		}
		return b.Receiver().(*Graph).AddNode(key, props, trace)
	}),

	// node(key: graph.key): graph.node
	"node": starlark.NewBuiltin("node", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var key *Key
		if err := starlark.UnpackArgs("node", args, kwargs, "key", &key); err != nil {
			return nil, err
		}
		if node := b.Receiver().(*Graph).Node(key); node != nil {
			return node, nil
		}
		return starlark.None, nil
	}),

	// add_edge(parent: graph.Key, child: graph.Key, title='', trace=stacktrace())
	"add_edge": starlark.NewBuiltin("add_edge", func(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var parent *Key
		var child *Key
		var title starlark.String
		var trace *builtins.CapturedStacktrace
		err := starlark.UnpackArgs("add_edge", args, kwargs,
			"parent", &parent,
			"child", &child,
			"title?", &title,
			"trace?", &trace)
		if err != nil {
			return nil, err
		}

		if trace == nil {
			var err error
			if trace, err = builtins.CaptureStacktrace(th, 0); err != nil {
				return nil, err
			}
		}

		err = b.Receiver().(*Graph).AddEdge(parent, child, title.GoString(), trace)
		return starlark.None, err
	}),
}
