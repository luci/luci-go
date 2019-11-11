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

type node struct {
	path     Path
	children map[string]*node
	adjacent []*node
	weight   float64
}

func (n *node) isTreeLeaf() bool {
	return len(n.children) == 0
}

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

func (n *node) remove(path Path) *node {
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
	from *node
	to   *node
}

type graph struct {
	root   node
	weight map[nodePair]float64
}

func (g *graph) visit(fn func(n *node) error) error {
	err := g.root.visit(fn)
	if err == errStop {
		err = nil
	}
	return err
}

func (g *graph) getNode(path Path) *node {
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

func (g *graph) moveLeaf(from, to Path) error {
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

func (g *graph) AddCommit(c commit) error {
	// Process change types and determine the set of files for which we will
	// adjust weights.
	type entry struct {
		path   Path
		weight int
	}
	toAdd := make([]entry, 0, len(c.Files))
	for _, f := range c.Files {
		e := entry{
			path:   f.Src,
			weight: f.Added,
		}

		switch f.Status {
		case 'D':
			// The file was deleted.
			g.moveLeaf(f.Src, nil)
			continue

		case 'R':
			// The file was renamed.
			g.moveLeaf(f.Src, f.Dst)
			e.path = f.Dst
		}

		if e.weight <= 0 {
			e.weight = 1
		}

		toAdd = append(toAdd, e)
	}

	if len(toAdd) > 100 {
		return nil
	}

	for i, e1 := range toAdd {
		n := g.ensureNode(e1.path)
		n.weight += float64(e1.weight)

		for j, e2 := range toAdd {
			if i == j {
				continue
			}
			n2 := g.ensureNode(e2.path)

			key := nodePair{n, n2}
			if g.weight == nil {
				g.weight = map[nodePair]float64{}
			}
			w, ok := g.weight[key]
			if !ok {
				n.adjacent = append(n.adjacent, n2)
			}
			g.weight[key] = w + float64(e1.weight)
		}
	}

	return nil
}
