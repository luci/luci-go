// Copyright 2021 The LUCI Authors.
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

import "sort"

type EdgeTag string

const (
	Owns EdgeTag = "OWNS"
	In   EdgeTag = "IN"
)

type NodeKind string

const (
	Identity NodeKind = "IDENTITY"
	Glob     NodeKind = "GLOB"
	Group    NodeKind = "GROUP"
)

// Subgraph is directed multigraph with labeled edges and a designated root node.
// Nodes are assigned integer IDs and edges are stored as a map
// {node_from_id => label => node_to_id}.
type Subgraph struct {
	// All nodes in Subgraph.
	Nodes []*SubgraphNode
	// Mapping of node to its id within the list of all nodes.
	nodesToID map[NodeKey]int
}

// SubgraphNode represents individual Nodes inside the Subgraph
type SubgraphNode struct {
	NodeKey
	// Contains edge connections to this node.
	Edges map[EdgeTag][]int
}

// NodeKey represents a key to identify Nodes.
type NodeKey struct {
	// Type of Node, (identity, group, glob).
	Kind NodeKind
	// Name of node, group-name usually.
	Value string
}

// addNode adds the given node if not present.
//
// returns nodeID and a bool representing whether it was added.
func (s *Subgraph) addNode(kind NodeKind, value string) (int, bool) {
	key := NodeKey{kind, value}

	// If Node is already present.
	if _, ok := s.nodesToID[key]; ok {
		return s.nodesToID[key], false
	}

	node := &SubgraphNode{
		NodeKey: key,
	}

	s.Nodes = append(s.Nodes, node)
	nodeID := len(s.Nodes) - 1
	s.nodesToID[key] = nodeID

	return nodeID, true
}

// addEdge adds an edge from(nodeID) to(nodeID) with the relation
// denoting whether the from node owns or is in the to node.
//
// returns true if the edge was successfully added. returns false
// if the edge was not added.
func (s *Subgraph) addEdge(from int, relation EdgeTag, to int) {
	if from < 0 || from >= len(s.Nodes) {
		panic("from is out of bounds")
	}

	if to < 0 || to >= len(s.Nodes) {
		panic("to is out of bounds")
	}

	// Specific node edges map.
	if s.Nodes[from].Edges == nil {
		s.Nodes[from].Edges = map[EdgeTag][]int{}
	}

	// Insert node in sorted order to maintain stability.
	s.Nodes[from].Edges[relation] = sortedInsert(s.Nodes[from].Edges[relation], to)
}

// sortedInsert inserts an element (v) into a slice in sorted
// order and returns a new slice with the element inserted.
// Does NOT allow duplicates. A helper function to addEdge().
// See https://stackoverflow.com/questions/42746972/golang-insert-to-a-sorted-slice.
func sortedInsert(data []int, v int) []int {
	i := sort.Search(len(data), func(i int) bool { return data[i] >= v })
	switch {
	case i < len(data) && data[i] == v:
		return data
	case i == len(data):
		return append(data, v)
	default:
		data = append(data[:i+1], data[i:]...)
		data[i] = v
		return data
	}
}
