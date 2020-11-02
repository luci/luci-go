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
//
// The weight of edge (x, y), called distance, represents how much y is relevant
// to x. It is a value between 0 and +inf, where 0 means extremely relevant
// and +inf means not relevant at all.
type Graph interface {
	// Node returns a node by its name.
	// Returns nil if the node is not found.
	// See also Node.Name().
	//
	// Idempotent: calling many times with the same name returns the same Node
	// object.
	Node(name string) Node
}

// Node is a node in a file graph.
// It is either a file or a directory.
type Node interface {
	// Name returns node's name.
	// It is an forward-slash-separated path with "//" prefix,
	// e.g. "//foo/bar.cc".
	Name() string

	// IsFile returns true if the node is a file, as opposed to a directory.
	IsFile() bool

	// Outgoing calls the given function for each direct successor.
	// If callback returns false, then iteration stops.
	Outgoing(callback func(other Node, distance float64) (stop bool))
}
