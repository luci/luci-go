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

import "math"

// Graph is a directed weigted graph of files.
type Graph interface {
	// Node returns a node at the path.
	Node(Path) Node
}

// Node is a node in the file graph.
// It can be a file or a directory.
type Node interface {
	// Path returns the path to the node.
	Path() Path

	// Adjacent calls the function for each node
	Adjacent(func(n Node, relevance float64) error) error

	// IsFile returns true if the node is a file, as opposed to a directory.
	IsFile() bool
}

// Distance returns distance between two nodes based on their relevance.
func Distance(relevance float64) float64 {
	switch {
	case relevance == 1:
		return 0
	case relevance < 0 || relevance > 1:
		panic("relevance must be between 0.0 and 1.0 inclusive")
	default:
		return -math.Log(relevance)
	}
}

// Relevance converts distance to relevance. It is the opposite of Distance().
func Relevance(distance float64) float64 {
	switch {
	case distance == 0:
		return 1
	case distance < 0:
		panic("distance cannot be negative")
	default:
		return math.Pow(math.E, -distance)
	}
}
