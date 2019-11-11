// Copyright 2019 The LUCI Authors.
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

package main

import (
	"go.chromium.org/luci/common/errors"
)

// graph is a graph of files where each node represents a file and edge (A, B)
// represents how much file B is relevant to file A.
//
// It is implemented as a trie which reflects the directory structure.
// Files are trie leafs, dirs are intermediate nodes.
// Only files have edges; dirs do not.
type graph struct {
	// the repository root
	root node

	// all edges in one hash map.
	// The nodes in the map key must be trie leafs.
	//
	// The map value is the non-normalized edge edgeWeights.
	// To normalize it, it must be divided by key.from.edgeWeights. See node.edgeWeights.
	//
	// Note: using one giant hash table with float64 values uses signifcantly less
	// memory that a hashtable in each node.
	edgeWeights map[nodePair]float64
}

// node is a trie node. See graph comment.
type node struct {
	// path to this file/dir.
	// Can be used to find ancestors by navigating from graph.root.
	path Path

	// files/subdirs in this dir.
	// If it is empty, then this node is a file (not dir) and MAY have edges.
	children map[string]*node

	// an unordered set of files that this file has directed edges to.
	// Must be nil for directories.
	adjacent []*node

	// the total number of added lines in all commits to this file.
	// Must be 0 for directories.
	weight float64
}

type nodePair struct {
	from *node
	to   *node
}

// isTreeLeaf returns true if the node has no children.
func (n *node) isTreeLeaf() bool {
	return len(n.children) == 0
}

// visit calls fn for all nodes, until all nodes are visited or fn returns an
// error.
// Use (*node).isTreeLeaf to determine if the node is a leaf.
// If fn returns errStop, visit returns nil.
func (g *graph) visit(fn func(n *node) error) error {
	err := g.root.visit(fn)
	if err == errStop {
		err = nil
	}
	return err
}

// visit implements graph.visit.
func (n *node) visit(fn func(n *node) error) error {
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

// ensureNode returns a node for the path if it exists; otherwise creates a new
// one.
func (g *graph) ensureNode(path Path) *node {
	cur := &g.root
	for i, c := range path {
		child := cur.children[c]
		if child == nil {
			child = &node{path: path[:i+1]}
			if cur.children == nil {
				cur.children = map[string]*node{}
			}
			cur.children[c] = child
		}
		cur = child
	}
	return cur
}

// AddCommit updates the graph according to the commit.
// This operation is irreversible.
// If the returned error is non-nil, then g is in the corrupted state and all
// future operations have undefined behavior.
func (g *graph) AddCommit(c commit) error {
	// Process change types and determine the set of files for which we will
	// adjust weights.
	type entry struct {
		path   Path
		weight float64
	}
	toAdd := make([]entry, 0, len(c.Files))
	for _, f := range c.Files {
		e := entry{
			path:   f.Src,
			weight: float64(f.Added),
		}

		switch f.Status {
		case deleted:
			// The file was deleted.
			// TODO(nodir): do not ignore the returned error.
			// We have to do this today because we don't process commits with two
			// parents correctly.
			g.moveLeaf(f.Src, nil)
			continue

		case renamed:
			// The file was renamed.
			// TODO(nodir): do not ignore the returned error.
			// We have to do this today because we don't process commits with two
			// parents correctly.
			g.moveLeaf(f.Src, f.Dst)
			e.path = f.Dst

			// TODO(nodir): add special handling for copied files.
		}

		// Use weight 1 for binary files.
		if e.weight <= 0 {
			e.weight = 1
		}

		toAdd = append(toAdd, e)
	}

	// Skip huge commits, such as refactoring, because they provide a weak signal
	// of relevancy. The reason we have to do this is that the loop below is
	// O(len(toAdd)^2).
	// TODO(nodir): also consider redusing the signal strength with the growth of
	// len(toAdd).
	// TODO(nodir): consider making this configurable.
	if len(toAdd) > 100 {
		return nil
	}

	// Update weights.
	for i, e1 := range toAdd {
		// For each entry, increase its weight.
		n := g.ensureNode(e1.path)
		n.weight += e1.weight

		// For each pair (e1, e2), increase the edge weight.
		for j, e2 := range toAdd {
			if i == j {
				continue // skip self
			}

			n2 := g.ensureNode(e2.path)
			key := nodePair{n, n2}
			if g.edgeWeights == nil {
				g.edgeWeights = map[nodePair]float64{}
			}
			w, ok := g.edgeWeights[key]
			if !ok {
				// This is a new edge. Update n.adjacent.
				n.adjacent = append(n.adjacent, n2)
			}
			g.edgeWeights[key] = w + e1.weight
		}
	}

	return nil
}

// moveLeaf moves a node from one path to another.
// The node being moved must be a leaf and the destination must not exist.
// Updates edges accordingly.
func (g *graph) moveLeaf(from, to Path) error {
	// First remove the node from the graph.
	n, err := g.root.removeLeaf(from)
	switch {
	case err != nil:
		return err
	case n == nil:
		return errors.Reason("%q not found", from).Err()
	}

	if len(to) == 0 {
		// We were asked to remove the node from the graph.
		// Remove the edges.
		for _, a := range n.adjacent {
			if err := a.removeAdjacent(n); err != nil {
				panic(errors.Annotate(err, "corrupted state").Err())
			}
			delete(g.edgeWeights, nodePair{n, a})
			delete(g.edgeWeights, nodePair{a, n})
		}
		return nil
	}

	// Attach the new path.
	n.path = to

	// Add it to another parent.
	newParentPath, newBase := to.Split()
	newParent := g.ensureNode(newParentPath)
	switch {
	case newParent.children == nil:
		newParent.children = map[string]*node{}
	case newParent.children[newBase] != nil:
		panic(errors.Reason("%q already exists", to).Err())
	}
	newParent.children[newBase] = n
	return nil
}

// removeAdjacent removes another from n.adjacent.
// Returns an error if another was not found in n.adjacent.
func (n *node) removeAdjacent(another *node) error {
	for i, a := range n.adjacent {
		if a == another {
			// Remove it
			copy(n.adjacent[i:], n.adjacent[i+1:])
			n.adjacent = n.adjacent[:len(n.adjacent)-1]
			return nil
		}
	}

	return errors.Reason("%q is not adjacent to %q", another.path, n.path).Err()
}

// remove removes a leaf node at path, where path is relative to n.
// If an ancestor of the removed node becomes empty, it is removed too, so that
// there dirs do not turn into files.
// Returns nil if the node to remove is not found.
func (n *node) removeLeaf(path Path) (*node, error) {
	if len(path) == 0 {
		panic("path is empty")
	}

	switch child := n.children[path[0]]; {
	case child == nil:
		return nil, nil

	case len(path) == 1:
		if !child.isTreeLeaf() {
			return nil, errors.Reason("%q is not a leaf", child.path).Err()
		}
		delete(n.children, path[0])
		return child, nil

	default:
		removed, err := child.removeLeaf(path[1:])
		if err != nil {
			return nil, err
		}
		if removed != nil && len(child.children) == 0 {
			// child was a dir and just lost its last child. Remove it.
			delete(n.children, path[0])
		}
		return removed, nil
	}
}
