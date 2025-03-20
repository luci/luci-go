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

package mvs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/lucicfg/internal"
)

// Stupid Go generics do not realize "string" is actually printable already,
// so we need to wrap it in a new type to implement String() for it.
type StringVer string

func (v StringVer) String() string { return string(v) }

type StringGraph = Graph[StringVer, string]
type StringPackage = Package[StringVer]
type StringDep = Dep[StringVer, string]

func TestGraph(t *testing.T) {
	t.Parallel()

	g, deps := buildGraph(t, `
		R1: A1 B1 C1
		A1: B2
		B1: C1
		B2: C2 A1
		C1:
		C2: C1
	`)

	sortedVers := func(pkg string) []string {
		var vers []string
		for _, v := range g.Versions(pkg) {
			vers = append(vers, string(v))
		}
		slices.Sort(vers)
		return vers
	}

	assert.That(t, g.Packages(), should.Match([]string{
		"A", "B", "C", "R",
	}))
	assert.That(t, sortedVers("R"), should.Match([]string{"1"}))
	assert.That(t, sortedVers("A"), should.Match([]string{"1"}))
	assert.That(t, sortedVers("B"), should.Match([]string{"1", "2"}))
	assert.That(t, sortedVers("C"), should.Match([]string{"1", "2"}))
	assert.That(t, sortedVers("X"), should.Match([]string(nil)))

	t.Run("Traverse along edges", func(t *testing.T) {
		var visited []string

		err := g.Traverse(func(n StringPackage, edges []StringDep) ([]StringPackage, error) {
			visited = append(visited, fmt.Sprintf("%s%s", n.Package, n.Version))
			assert.That(t, edges, should.Match(deps[n]))
			if len(edges) == 0 {
				return nil, nil
			}
			return []StringPackage{edges[0].Package}, nil
		})

		assert.NoErr(t, err)
		assert.That(t, visited, should.Match([]string{
			"R1", "A1", "B2", "C2", "C1",
		}))
	})

	t.Run("Traverse hopping", func(t *testing.T) {
		var visited []string

		err := g.Traverse(func(n StringPackage, edges []StringDep) ([]StringPackage, error) {
			visited = append(visited, fmt.Sprintf("%s%s", n.Package, n.Version))
			var next []StringPackage
			for _, edge := range edges {
				// Pick the largest available version.
				vers := sortedVers(edge.Package.Package)
				next = append(next, StringPackage{
					Package: edge.Package.Package,
					Version: StringVer(vers[len(vers)-1]),
				})
			}
			return next, nil
		})

		assert.NoErr(t, err)
		assert.That(t, visited, should.Match([]string{
			"R1", "C2", "B2", "A1",
		}))
	})

	t.Run("Traverse err", func(t *testing.T) {
		var visited []string

		boomErr := errors.New("BOOM")

		err := g.Traverse(func(n StringPackage, edges []StringDep) ([]StringPackage, error) {
			visited = append(visited, fmt.Sprintf("%s%s", n.Package, n.Version))
			if len(edges) == 0 {
				return nil, nil
			}
			if len(visited) == 4 {
				return nil, boomErr
			}
			return []StringPackage{edges[0].Package}, nil
		})

		assert.That(t, err, should.Equal(boomErr))
		assert.That(t, visited, should.Match([]string{
			"R1", "A1", "B2", "C2",
		}))
	})

	t.Run("Traverse unknown node", func(t *testing.T) {
		var visited []string

		err := g.Traverse(func(n StringPackage, edges []StringDep) ([]StringPackage, error) {
			visited = append(visited, fmt.Sprintf("%s%s", n.Package, n.Version))
			if len(edges) == 0 {
				return nil, nil
			}
			if len(visited) == 4 {
				return []StringPackage{
					{
						Package: edges[0].Package.Package,
						Version: "9",
					},
				}, nil
			}
			return []StringPackage{edges[0].Package}, nil
		})

		assert.That(t, err, should.ErrLike(`"C, rev 2" attempts to visit non-existing node "C, rev 9"`))
		assert.That(t, visited, should.Match([]string{
			"R1", "A1", "B2", "C2",
		}))
	})
}

func buildGraph(t *testing.T, spec string) (*StringGraph, map[StringPackage][]StringDep) {
	t.Helper()

	root, deps := parseDeps(spec)

	// Feed all `deps` to the graph. Usually they will be fetched lazily right
	// during the traversal. Here we also feed it concurrently, but mostly just
	// to hit the relevant code paths and to tickle the race detector.
	g := NewGraph[StringVer, string](root)
	wq, _ := internal.NewWorkQueue[StringPackage](context.Background())
	wq.Launch(func(pkg StringPackage) error {
		if _, ok := deps[pkg]; !ok {
			panic(fmt.Sprintf("asked to visit unexpected package %s", pkg))
		}
		for _, next := range g.Require(pkg, deps[pkg]) {
			wq.Submit(next.Package)
		}
		return nil
	})
	wq.Submit(root)
	assert.NoErr(t, wq.Wait())
	assert.That(t, g.Finalize(), should.BeTrue)
	return g, deps
}

func parseDeps(spec string) (root StringPackage, deps map[StringPackage][]StringDep) {
	pkg := func(spec string) StringPackage {
		spec = strings.TrimSpace(spec)
		return StringPackage{
			Package: spec[:1],            // e.g. "A"
			Version: StringVer(spec[1:]), // e.g. "2"
		}
	}

	deps = map[StringPackage][]StringDep{}

	first := true
	for _, line := range strings.Split(spec, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		srcSpec, depsSpec, _ := strings.Cut(line, ":")
		srcSpec = strings.TrimSpace(srcSpec)
		depsSpec = strings.TrimSpace(depsSpec)

		srcPkg := pkg(srcSpec)
		if first {
			root = srcPkg
			first = false
		}

		var edges []StringDep
		if depsSpec != "" {
			for _, depSpec := range strings.Split(depsSpec, " ") {
				depPkg := pkg(depSpec)
				edges = append(edges, StringDep{
					Package: depPkg,
					Meta:    fmt.Sprintf("%s->%s", srcSpec, depSpec),
				})
			}
		}
		deps[srcPkg] = edges
	}

	return
}
