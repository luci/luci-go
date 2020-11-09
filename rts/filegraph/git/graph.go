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

package git

import (
	"math"
	"sort"
	"strings"
	"sync"

	"go.chromium.org/luci/rts/filegraph"
)

// Graph is a file graph based on the git history.
type Graph struct {
	// Commit is the git commit that the graph state corresponds to.
	Commit string

	root node
	init sync.Once
}

// node is simultaneously a distance graph node (see edges) and a filesystem
// tree node (see children).
// It implements filegraph.Node.
type node struct {
	// name is the node name, e.g. "//foo/bar.cc"
	// See also filegraph.Node.Name().
	name string

	// commits is the number of commits that touched this node.
	commits int

	// edges are outgoing edges.
	// If an edge exists from x to y, then it must also exist from y to x and must
	// have the same edge.commonCommits.
	//
	// Note: this data structure is optimized for the Dijkstra's algorithm
	// and loading from disk. None of them need random-access.
	edges []edge

	// copyEdgesOnAppend indicates that edges must be copied before appending.
	copyEdgesOnAppend bool

	// children are files and subdirectories of the current node, which itself
	// is a directory.
	children map[string]*node
}

type edge struct {
	to *node

	// commonCommits is the number of commits that touch both edge nodes.
	//
	// Sentinel value 0 means that the destination node is the alias of the
	// source node, and the distance is 0.
	// It is used for file renames. In such case, the old and the new file are
	// aliases to each other.
	commonCommits int
}

func (g *Graph) ensureInitialized() {
	g.init.Do(func() {
		g.root.name = "//"
	})
}

// Node implements filegraph.Graph.
func (g *Graph) Node(name string) filegraph.Node {
	g.ensureInitialized()
	return g.node(name)
}

// node retrieves a graph node by name. Returns nil if it doesn't exist.
func (g *Graph) node(name string) *node {
	cur := &g.root
	for _, component := range splitName(name) {
		if cur = cur.children[component]; cur == nil {
			return nil
		}
	}
	return cur
}

// ensureNode creates the node if it doesn't exist, and returns it.
// Creates the node's ancestors if needed.
// Assumes the name is valid.
func (g *Graph) ensureNode(name string) *node {
	if name == "//" {
		return &g.root
	}

	cur := &g.root

	child := func(baseName, name string) *node {
		if ret, ok := cur.children[baseName]; ok {
			return ret
		}

		ret := &node{name: name}
		if cur.children == nil {
			cur.children = map[string]*node{}
		}
		cur.children[baseName] = ret
		return ret
	}

	startAt := 2 // skip the "//" prefix
	for {
		sep := strings.Index(name[startAt:], "/")
		if sep == -1 {
			return child(name[startAt:], name)
		}
		sep += startAt

		// Note: no string allocations for the full name.
		cur = child(name[startAt:sep], name[:sep])
		startAt = sep + 1
	}
}

func (n *node) ensureAlias(to *node) {
	for i := range n.edges {
		if n.edges[i].to == to {
			n.edges[i].commonCommits = 0
			return
		}
	}

	n.prepareToAppendEdges()
	n.edges = append(n.edges, edge{to: to})
}

func (n *node) Name() string {
	return n.name
}

func (n *node) IsFile() bool {
	return len(n.children) == 0 && n.name != "//"
}

func (n *node) Outgoing(callback func(other filegraph.Node, distance float64) (keepGoing bool)) {
	for _, e := range n.edges {
		distance := 0.0
		if e.commonCommits == 0 {
			// e.to is alias of n. The distance is 0.
		} else {
			// TODO(nodir): consider using multiplication in filegraph.Query instead of
			// calling log2, because the latter is expensive.
			distance = -math.Log2(float64(e.commonCommits) / float64(n.commits))
		}
		if !callback(e.to, distance) {
			return
		}
	}
}

// visit calls callback for each node in the subtree rooted at n.
// If the callback returns false for a node, then its descendants are not
// visited.
func (n *node) visit(callback func(*node) bool) {
	if !callback(n) {
		return
	}

	for _, child := range n.children {
		child.visit(callback)
	}
}

func (n *node) sortedChildKeys() []string {
	if len(n.children) == 0 {
		return nil
	}
	keys := make([]string, 0, len(n.children))
	for name := range n.children {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

// prepareToAppendEdges copies n.edges if n.copyEdgesOnAppend is true.
func (n *node) prepareToAppendEdges() {
	if !n.copyEdgesOnAppend {
		return
	}

	n.copyEdgesOnAppend = false
	// Double the capacity in preparation for the append.
	edges := make([]edge, len(n.edges), len(n.edges)*2)
	copy(edges, n.edges)
	n.edges = edges
}

// splitName splits a node name into components,
// e.g. "//foo/bar.cc" -> ["foo", "bar.cc"].
func splitName(name string) []string {
	name = strings.TrimPrefix(name, "//")
	if name == "" {
		return nil
	}
	return strings.Split(name, "/")
}
