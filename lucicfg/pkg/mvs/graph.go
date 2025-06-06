// Copyright 2025 The LUCI Authors.
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

// Package mvs implements a graph used by "Minimal Version Selection" algorithm.
//
// See https://research.swtch.com/vgo-mvs.
package mvs

import (
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"go.chromium.org/luci/common/errors"
)

// Version represents a particular version of a particular package.
//
// Can be anything as long as we can use it as a map key (to skip duplicates).
//
// It is callers responsibility to eventually figure out what version is the
// most recent. The graph itself doesn't care, it collects all distinct
// versions.
type Version interface {
	comparable

	// String is used for debug-printing this version in logs and errors.
	String() string
}

// Package is a (Name, Version) pair acting as a node in the graph.
type Package[V Version] struct {
	Package string // a package name
	Version V      // its version
}

// String is used for debug-printing this package in logs and errors.
func (p Package[V]) String() string {
	return fmt.Sprintf("%s, rev %s", p.Package, p.Version)
}

// Graph contains a full dependency graph across all versions of dependencies.
//
// Nodes of the graph are (Name, Version) tuples (see Package struct). Once the
// graph is built, we'll examine it to select most recent versions of all
// packages as the final version set.
//
// Cycles are permitted.
//
// Nodes can be in two states: unexplored and explored. All new nodes are
// initially in unexplored state. A node switches into explored state via
// Require call. That call may add new unexplored nodes via Deps.
//
// The graph is built when all its nodes are explored. A built graph can be
// finalized via Finalize call. A finalized graph can be examined and traversed.
//
// All methods can be called concurrently.
type Graph[V Version] struct {
	m         sync.Mutex
	root      Package[V]                  // the root package
	pkgs      map[string]map[V]struct{}   // package name => the set of its observed versions
	deps      map[Package[V]][]Package[V] // package version => its direct dependencies
	explored  map[Package[V]]bool         // package version => true if already explored it
	finalized atomic.Bool                 // true if mutations are forbidden already
}

// NewGraph creates a new graph, seeding it with the given root package.
//
// The root is initially in unexplored state (i.e. you'll need to call Require
// for it).
func NewGraph[V Version](root Package[V]) *Graph[V] {
	return &Graph[V]{
		root:     root,
		pkgs:     make(map[string]map[V]struct{}, 1),
		deps:     make(map[Package[V]][]Package[V], 1),
		explored: map[Package[V]]bool{root: false},
	}
}

// Require declares that the given unexplored Package node has given direct
// dependencies.
//
// All packages specified in `deps` will be added to the graph as nodes (if not
// already there). All newly added nodes will be in unexplored state and this
// Require call will return a subset of `deps` with dependencies to new
// unexplored nodes (assuming the caller will eventually call Require to explore
// them as well).
//
// Panics if `pkg` is not in the graph or if it was already explored. Also
// panics if the graph was finalized already.
func (g *Graph[V]) Require(pkg Package[V], deps []Package[V]) []Package[V] {
	g.m.Lock()
	defer g.m.Unlock()

	if g.finalized.Load() {
		panic("cannot modify finalized graph")
	}

	switch explored, seen := g.explored[pkg]; {
	case !seen:
		panic(fmt.Sprintf("%q is not in the graph", pkg))
	case explored:
		panic(fmt.Sprintf("%q was already explored", pkg))
	}

	g.observeVersionLocked(pkg)
	g.deps[pkg] = slices.Clip(deps)
	g.explored[pkg] = true

	var explore []Package[V]
	for _, dep := range deps {
		if _, seen := g.explored[dep]; seen {
			continue
		}
		// New node, make it unexplored.
		g.explored[dep] = false
		explore = append(explore, dep)
	}
	return explore
}

// Finalize verifies all nodes are explored and, if so, forbids any further
// modifications of the graph.
//
// Returns true if all nodes are explored or false if some were left unexplored.
func (g *Graph[V]) Finalize() bool {
	g.m.Lock()
	defer g.m.Unlock()
	for _, yes := range g.explored {
		if !yes {
			return false
		}
	}
	g.finalized.Store(true)
	return true
}

// Packages returns names of all packages referenced in the graph.
//
// They are sorted lexicographically.
//
// The graph must be finalized already.
func (g *Graph[V]) Packages() []string {
	g.panicNonFinalized()
	return slices.Sorted(maps.Keys(g.pkgs))
}

// Versions returns all referenced versions of the given package.
//
// They are returned in some arbitrary non-deterministic order.
//
// The graph must be finalized already.
func (g *Graph[V]) Versions(pkg string) []V {
	g.panicNonFinalized()
	return slices.Collect(maps.Keys(g.pkgs[pkg]))
}

// Traverse visits the graph nodes starting from the root.
//
// The callback receives the node being visited along with all its edges. It
// returns what nodes to visit next (or an error). The returned nodes don't have
// to be connected by an edge to the current node, but they must exist in the
// graph. Returned nodes that haven't been visited already will be visited at
// some point. The traversal happens in the depth-first order.
//
// The graph must be finalized already.
func (g *Graph[V]) Traverse(v func(node Package[V], edges []Package[V]) ([]Package[V], error)) error {
	g.panicNonFinalized()

	visited := make(map[Package[V]]struct{})
	stack := make([]Package[V], 0, len(g.deps))

	// This is DFS_iterative from https://en.wikipedia.org/wiki/Depth-first_search
	stack = append(stack, g.root)
	for len(stack) > 0 {
		// Pop.
		var cur Package[V]
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		if _, yes := visited[cur]; yes {
			continue
		}

		// Visit.
		visited[cur] = struct{}{}
		deps, ok := g.deps[cur]
		if !ok {
			panic("impossible, already checked when adding to the stack")
		}
		next, err := v(cur, deps)
		if err != nil {
			return err
		}

		// Verify the callback returned existing nodes and schedule future visits.
		for _, pkg := range next {
			if _, yes := visited[pkg]; yes {
				continue
			}
			if _, exists := g.deps[pkg]; !exists {
				return errors.Fmt("%q attempts to visit non-existing node %q", cur, pkg)
			}
			stack = append(stack, pkg)
		}
	}

	return nil
}

// panicNonFinalized panics if the graph is not finalized yet.
func (g *Graph[V]) panicNonFinalized() {
	if !g.finalized.Load() {
		panic("the graph is not finalized")
	}
}

// observeVersionLocked puts the version into g.pkgs.
//
// Called under the lock.
func (g *Graph[V]) observeVersionLocked(pkg Package[V]) {
	cur := g.pkgs[pkg.Package]
	if cur == nil {
		cur = make(map[V]struct{}, 1)
		g.pkgs[pkg.Package] = cur
	}
	cur[pkg.Version] = struct{}{}
}
