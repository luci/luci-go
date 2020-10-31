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

import (
	"go.chromium.org/luci/common/errors"
)

// Dijkstra imeplements Dijkstra's algorithm to find shortest paths to
// other nodes, producing a shortest-path tree.
//
// See also:
//  https://en.wikipedia.org/wiki/Dijkstra%27s_algorithm
//  https://en.wikipedia.org/wiki/Shortest-path_tree
type Dijkstra struct {
	Sources []Node
	// TODO(crbug.com/1136280): add Backwards
	// TODO(crbug.com/1136280): add MaxDistance
	// TODO(crbug.com/1136280): add SilbingDistance
}

// ShortestPath is a node in a shortest path tree,
// representing the shortest path from one of Dijkstra.Sources
// to ShortestPath.Node.
// See also https://en.wikipedia.org/wiki/Shortest-path_tree
type ShortestPath struct {
	Node     Node
	Distance float64
	// Prev points to the previous node on the path from a source to this node.
	Prev *ShortestPath
}

// ErrStop when returned by a callback indicates that an iteration must stop.
var ErrStop = errors.New("stop the iteration")

// Run calls the callback for each node reachable from any of the sources.
// The nodes are reported in the ascending distance order relative to the
// sources, starting from the sources themselves.
//
// If the callback returns ErrStop, then the iteration stops.
func (q *Dijkstra) Run(g Graph, callback func(*ShortestPath) error) error {
	// TODO(1136280): implement.
	panic("not implemented")
}
