// Copyright 2020 The LUCI Authors.
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

package filegraph

// Graph is a directed weighted graph of files.
// The weight of edge (x, y) represents how much y is relevant to x.
type Graph interface {
	// Node returns a node by its name.
	// Returns nil if the node is not found.
	Node(Name) Node
}

// Node is a node in a file graph.
// It might be a file or a directory.
type Node interface {
	// Name returns node's name.
	Name() Name

	// IsFile returns true if the node is a file, as opposed to a directory.
	IsFile() bool

	// Adjacent calls the given function for each adjacent node.
	// Respects the direction: distance represents the distance from the current
	// node to n.
	Adjacent(func(n Node, distance float64) error) error
}
