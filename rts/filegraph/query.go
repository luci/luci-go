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

// Query finds shortest paths from any of Query.Sources to other nodes,
// producing a shortest-path tree
// (https://en.wikipedia.org/wiki/Shortest-path_tree).
//
// It is based on Dijkstra's algorithm
// (https://en.wikipedia.org/wiki/Dijkstra's_algorithm).
type Query struct {
	// Sources are the nodes to start from.
	Sources []Node

	// TODO(crbug.com/1136280): add Backwards
	// TODO(crbug.com/1136280): add MaxDistance
	// TODO(crbug.com/1136280): add SilbingDistance
}

// ShortestPath represents the shortest path from one of sources to a node.
// ShortestPath itself is a node in a shortest path tree
// (https://en.wikipedia.org/wiki/Shortest-path_tree).
type ShortestPath struct {
	Node     Node
	Distance float64
	// Prev points to the previous node on the path from a source to this node.
	// It can be used to reconstruct the whole shortest path, starting from a
	// source.
	Prev *ShortestPath
}

// Run calls the callback for each node reachable from any of the sources.
// The nodes are reported in the distance-ascending order relative to the
// sources, starting from the sources themselves.
//
// If the callback returns false, then the iteration stops.
func (q *Query) Run(callback func(*ShortestPath) (stop bool)) {
	// TODO(crbug.com/1136280): implement.
	panic("not implemented")
}
