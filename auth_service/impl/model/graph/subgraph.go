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

import (
	"fmt"
	"sort"

	"go.chromium.org/luci/auth_service/api/rpcpb"
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
	nodesToID map[NodeKey]int32
}

// SubgraphNode represents individual Nodes inside the Subgraph
type SubgraphNode struct {
	NodeKey

	// IncludedBy represents nodes that include this node.
	IncludedBy []int32
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
func (s *Subgraph) addNode(kind NodeKind, value string) (int32, bool) {
	key := NodeKey{kind, value}

	// If Node is already present.
	if _, ok := s.nodesToID[key]; ok {
		return s.nodesToID[key], false
	}

	node := &SubgraphNode{
		NodeKey: key,
	}

	s.Nodes = append(s.Nodes, node)
	nodeID := int32(len(s.Nodes) - 1)
	s.nodesToID[key] = nodeID

	return nodeID, true
}

// addEdge adds an edge from(nodeID) to(nodeID).
//
// returns true if the edge was successfully added. returns false
// if the edge was not added.
func (s *Subgraph) addEdge(from int32, to int32) {
	if from < 0 || from >= int32(len(s.Nodes)) {
		panic("from is out of bounds")
	}

	if to < 0 || to >= int32(len(s.Nodes)) {
		panic("to is out of bounds")
	}

	// Specific node edges map.
	if s.Nodes[from].IncludedBy == nil {
		s.Nodes[from].IncludedBy = []int32{}
	}

	// Insert node in sorted order to maintain stability.
	s.Nodes[from].IncludedBy = sortedInsert(s.Nodes[from].IncludedBy, to)
}

// sortedInsert inserts an element (v) into a slice in sorted
// order and returns a new slice with the element inserted.
// Does NOT allow duplicates. A helper function to addEdge().
// See https://stackoverflow.com/questions/42746972/golang-insert-to-a-sorted-slice.
func sortedInsert(data []int32, v int32) []int32 {
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

// ToProto converts the SubgraphNode to the protobuffer
// equivalent Node for rpc.
func (sn *SubgraphNode) ToProto() *rpcpb.Node {
	return &rpcpb.Node{
		Principal:  sn.NodeKey.ToProto(),
		IncludedBy: sn.IncludedBy,
	}
}

// ToPermissionKey returns the key that would be associated with this node in a
// realms config.
func (nk *NodeKey) ToPermissionKey() string {
	var prefix string
	switch nk.Kind {
	case Group:
		prefix = "group:"
	case Identity, Glob:
		prefix = ""
	default:
		return ""
	}

	return fmt.Sprintf("%s%s", prefix, nk.Value)
}

// ToProto converts the NodeKey for the internal subgraph representation
// to the protobuffer equivalent Principal for rpc.
func (nk *NodeKey) ToProto() *rpcpb.Principal {
	return &rpcpb.Principal{
		Kind: func() rpcpb.PrincipalKind {
			switch nk.Kind {
			case Identity:
				return rpcpb.PrincipalKind_IDENTITY
			case Glob:
				return rpcpb.PrincipalKind_GLOB
			case Group:
				return rpcpb.PrincipalKind_GROUP
			default:
				return rpcpb.PrincipalKind_PRINCIPAL_KIND_UNSPECIFIED
			}
		}(),
		Name: nk.Value,
	}
}

// ToProto converts the Subgraph to the protobuffer
// equivalent Subgraph for rpc.
func (s *Subgraph) ToProto() *rpcpb.Subgraph {
	nodes := make([]*rpcpb.Node, len(s.Nodes))
	for idx, val := range s.Nodes {
		nodes[idx] = val.ToProto()
	}

	return &rpcpb.Subgraph{
		Nodes: nodes,
	}
}
