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
//
// The graph represents aggregated history of all file changes in the repo,
// rather than the state of the repo at a single point of time.
// In particular, the graph may include nodes for files that no longer exist.
// It is generally not possible to tell if a node is a file or a directory,
// because it might have been a file at one point of time, and a directory at
// another.
//
// TODO(nodir): introduce a decay function to remove old nodes/edges.
type Graph struct {
	// Commit is the git commit that the graph state corresponds to.
	Commit string

	root node
	init sync.Once
}

// EdgeReader implements filegraph.EdgeReader.
// It works only with nodes returned by Graph.Node().
type EdgeReader struct {
	// Reversed indicates that incoming edges must be read instead of outgoing.
	// In other words, read the edges of the tranposed graph.
	Reversed bool

	// TODO(nodir): add FamilyDistance.
}

// node is simultaneously a distance graph node (see edges) and a filesystem
// tree node (see children).
// It implements filegraph.Node.
//
// A node represents aggregated change history of a single node path.
// It is never excluded from the graph because the past is immutable.
// If a node was a file at one point, and a directory at another, it has
// both children and edges.
type node struct {
	// name is the node name, e.g. "//foo/bar.cc"
	// See also filegraph.Node.Name().
	name string

	// commits is the number of commits that touched this file.
	// Note that if "//foo/bar.cc" is touched, foo's commit count is
	// not incremented.
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

	// children are files and subdirectories of the this directory.
	// TODO(nodir): consider a sorted list instead.
	children map[string]*node
}

// edge is directed edge.
//
// If an edge exists from x to y, then a counterpart edge from y to x must also
// exist and have the same commonCommits.
//
// A special kind of edges is called "alias edge". It is indicated by
// commonCommits == 0. If edge (x, y) is an alias, then distance(x, y) is 0.
// Alias edges are used for file renames: the old and the new file are aliases
// of each other.
// Alias edges are never downgraded to regular edges - they stay alias because
// distance 0 is the minimal possible distance.
type edge struct {
	to            *node
	commonCommits int
}

func (g *Graph) ensureInitialized() {
	g.init.Do(func() {
		g.root.name = "//"
	})
}

// Node returns a node by its name.
// Returns nil if the node is not found.
// See also Node.Name().
//
// Idempotent: calling many times with the same name returns the same Node
// object.
func (g *Graph) Node(name string) filegraph.Node {
	g.ensureInitialized()
	if n := g.node(name); n != nil {
		return n
	}
	return nil
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

	// Note: this loop does not allocate new strings.
	startAt := 2 // skip the "//" prefix
	for {
		sep := strings.Index(name[startAt:], "/")
		if sep == -1 {
			return child(name[startAt:], name)
		}
		sep += startAt

		cur = child(name[startAt:sep], name[:sep])
		startAt = sep + 1
	}
}

func (n *node) Name() string {
	return n.name
}

// ReadEdges implements filegraph.EdgeReader.
func (r *EdgeReader) ReadEdges(from filegraph.Node, callback func(to filegraph.Node, distance float64) (keepGoing bool)) {
	n := from.(*node)
	outgoing := !r.Reversed
	for _, e := range n.edges {
		distance := 0.0
		if e.commonCommits == 0 {
			// e.to is alias of n. The distance is 0.
		} else {
			var sampleSpaceSize int // https://en.wikipedia.org/wiki/Sample_space
			if outgoing {
				sampleSpaceSize = n.commits
			} else {
				sampleSpaceSize = e.to.commits
			}

			// TODO(nodir): consider using multiplication in filegraph.Query instead of
			// calling log2, because the latter is expensive.

			// Note: commonCommits is same for incoming and outgoing edges.
			distance = -math.Log2(float64(e.commonCommits) / float64(sampleSpaceSize))
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

// ensureAlias ensures there is an alias edge from n to `to`.
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

// prepareToAppendEdges copies n.edges if n.copyEdgesOnAppend is true.
func (n *node) prepareToAppendEdges() {
	if !n.copyEdgesOnAppend {
		return
	}

	if len(n.edges) > 0 {
		// Double the capacity in preparation for the append.
		edges := make([]edge, len(n.edges), len(n.edges)*2)
		copy(edges, n.edges)
		n.edges = edges
	}

	n.copyEdgesOnAppend = false
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
