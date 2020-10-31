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

// Graph is a directed weighted graph of files.
// The weight of edge (x, y) represents how much y is relevant to x.
type Graph interface {
	// Node returns a node by its name.
	// Returns nil if the node is not found.
	Node(Name) Node
}

// Node is a node in the file graph.
// It can be a file or a directory.
type Node interface {
	// Name returns node's name.
	Name() Name

	// Adjacent calls the given function for each adjacent node.
	// Must respect the direction: relevance must represent weight of the edge
	// (self, n) and not (n, self).
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
