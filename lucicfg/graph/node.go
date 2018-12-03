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
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"go.chromium.org/luci/starlark/builtins"
)

// Node is an element of the graph.
type Node struct {
	Key   *Key                         // unique ID of the node
	Props *starlarkstruct.Struct       // struct(...) with frozen properties
	Trace *builtins.CapturedStacktrace // where the node was defined
}

// String is a part of starlark.Value interface.
func (n *Node) String() string {
	// TODO(vadimsh): Return something more useful.
	return "graph.node"
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
