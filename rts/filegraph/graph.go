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
	"sort"

	"go.chromium.org/luci/common/errors"
)

// Node represent a file.
type Node struct {
	// Path is the file path.
	// Do not modify.
	Path     Path
	children map[string]*Node
	adjacent []*Node
	weight   float64
}

// IsTreeLeaf returns true if n is a file, as opposed to a directory.
func (n *Node) IsTreeLeaf() bool {
	return len(n.children) == 0
}

func (n *Node) removeAdjacent(another *Node) error {
	for i, a := range n.adjacent {
		if a == another {
			// Remove it
			copy(n.adjacent[i:], n.adjacent[i+1:])
			n.adjacent = n.adjacent[:len(n.adjacent)-1]
			return nil
		}
	}

	return errors.Reason("%q is not adjacent to %q", another.Path, n.Path).Err()
}

func (n *Node) visit(fn func(n *Node) error) error {
	if err := fn(n); err != nil {
		return err
	}
	for _, c := range n.children {
		if err := c.visit(fn); err != nil {
			return err
		}
	}
	return nil
}

func (n *Node) visitDeterministicOrder(fn func(n *Node) error) error {
	if err := fn(n); err != nil {
		return err
	}

	names := make([]string, 0, len(n.children))
	for name := range n.children {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := n.children[name].visitDeterministicOrder(fn); err != nil {
			return err
		}
	}
	return nil
}

func (n *Node) remove(path Path) *Node {
	if len(path) == 0 {
		panic("path is empty")
	}

	switch child := n.children[path[0]]; {
	case child == nil:
		return nil

	case len(path) == 1:
		delete(n.children, path[0])
		return child

	default:
		removed := child.remove(path[1:])
		if removed != nil && len(child.children) == 0 {
			delete(n.children, path[0])
		}
		return removed
	}
}

type nodePair struct {
	from *Node
	to   *Node
}

// Graph is a directed weigted graph of files.
// The weight of edge (A, B) represents the strength of assoication from file A
// to file B.
type Graph struct {
	root   Node
	weight map[nodePair]float64
}

func (g *Graph) visit(fn func(n *Node) error) error {
	err := g.root.visit(fn)
	if err == ErrStop {
		err = nil
	}
	return err
}

// Node returns an existing node with the given path, or nil if not found.
func (g *Graph) Node(path Path) *Node {
	cur := &g.root
	for _, c := range path {
		child := cur.children[c]
		if child == nil {
			return nil
		}
		cur = child
	}
	return cur
}

func (g *Graph) ensureNode(path Path) *Node {
	cur := &g.root
	for i, c := range path {
		child := cur.children[c]
		if child == nil {
			child = &Node{Path: path[:i+1]}
			if cur.children == nil {
				cur.children = map[string]*Node{}
			}
			cur.children[c] = child
		}
		cur = child
	}
	return cur
}

func (g *Graph) moveLeaf(from, to Path) error {
	n := g.root.remove(from)
	switch {
	case n == nil:
		return errors.Reason("not found").Err()

	case len(n.children) > 0:
		panic("removed a dir")
	}

	if len(to) == 0 {
		// Remove the node.

		// Unlink the node.
		for _, a := range n.adjacent {
			if err := a.removeAdjacent(n); err != nil {
				panic(errors.Annotate(err, "corrupted state").Err())
			}
			delete(g.weight, nodePair{n, a})
			delete(g.weight, nodePair{a, n})
		}
		return nil
	}

	n.Path = to
	// Add it to another parent.
	newParentPath, newBase := to.Split()
	newParent := g.ensureNode(newParentPath)
	switch {
	case newParent.children == nil:
		newParent.children = map[string]*Node{}
	case newParent.children[newBase] != nil:
		panic(errors.Reason("%q already exists", to).Err())
	}
	newParent.children[newBase] = n
	return nil
}

func (g *Graph) addCommit(c commit) error {
	if g.weight == nil {
		g.weight = map[nodePair]float64{}
	}

	// Process change types and determine the set of files for which we will
	// adjust weights.
	// type entry struct {
	// 	path   Path
	// 	weight int
	// }
	// toAdd := make([]entry, 0, len(c.Files))
	toAdd := make([]Path, 0, len(c.Files))
	for _, f := range c.Files {
		// e := entry{
		// 	path:   f.Path,
		// 	weight: f.Added,
		// }

		path := f.Path
		switch f.Status {
		case 'D':
			// The file was deleted.
			g.moveLeaf(f.Path, nil)
			continue

		case 'R':
			// The file was renamed.
			g.moveLeaf(f.Path, f.Path2)
			path = f.Path2
		}

		// if e.weight <= 0 {
		// 	e.weight = 1
		// }

		// toAdd = append(toAdd, e)
		toAdd = append(toAdd, path)
	}

	// TODO(nodir): add a comment explaining this.
	if len(toAdd) > 100 || len(toAdd) == 1 {
		return nil
	}

	nodes := make([]*Node, len(toAdd))
	for i, path := range toAdd {
		nodes[i] = g.ensureNode(path)
	}

	for _, n := range nodes {
		// n.weight += float64(e1.weight)
		n.weight += 1.0

		for _, n2 := range nodes {
			if n == n2 {
				continue
			}

			// TODO(nodir): Consider another loop over path prefixes
			// to better support
			// chrome/browser/ssl/ssl_browsertest.cc -> chrome/browser/interstitials/chrome_settings_page_helper.c

			key := nodePair{n, n2}
			w, ok := g.weight[key]
			if !ok {
				n.adjacent = append(n.adjacent, n2)
			}
			// TODO(nodir): take into account the number of modified files.
			g.weight[key] = w + 1.0
			// g.weight[key] = w + float64(e1.weight)
		}
	}

	return nil
}
