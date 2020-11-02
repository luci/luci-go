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
	"path"
	"sort"
	"strings"

	"go.chromium.org/luci/rts/filegraph"
)

// Graph is a file graph based on the git history.
// It is stored on the file system and isn't load into memory entirely.
type Graph struct {
	root node
	// commit is the git commit that the graph state corresponds to.
	commit string
}

type node struct {
	name     string
	children map[string]*node
	edges    []edge
	commits  int
}

type edge struct {
	other         *node
	commonCommits int
}

func (g *Graph) ensureInitialized() {
	if g.root.name != "" {
		return
	}

	g.root.name = "//"
	g.root.children = map[string]*node{}
}

// Node implements filegraph.Graph.
func (g *Graph) Node(name string) filegraph.Node {
	g.ensureInitialized()
	return g.node(name)
}

func (g *Graph) node(name string) *node {
	cur := &g.root
	for _, comp := range splitName(name) {
		if cur = cur.children[comp]; cur == nil {
			return nil
		}
	}
	return cur
}

func (n *node) Name() string {
	return n.name
}

func (n *node) IsFile() bool {
	return len(n.children) == 0
}

func (n *node) Adjacent(outgoingEdges bool, callback func(other filegraph.Node, distance float64) bool) {
	for _, e := range n.edges {
		denominator := n.commits
		if !outgoingEdges {
			denominator = e.other.commits
		}

		// TODO(nodir): consider use multiplication in filegraph.Query instead of
		// calling log2, because the latter is expensive.

		// The number of common commits is same on both incoming and outgoing edges.
		distance := -math.Log2(float64(e.commonCommits) / float64(denominator))
		if !callback(e.other, distance) {
			return
		}
	}
}

func (n *node) sortedChildKeys() []string {
	keys := make([]string, 0, len(n.children))
	for name := range n.children {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

func splitName(name string) []string {
	name = strings.TrimPrefix(name, "//")
	return strings.Split(name, "/")
}

func joinName(parts ...string) string {
	return "//" + path.Join(parts...)
}
