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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type testGraph struct {
	nodes map[string]*testNode
}

func (g *testGraph) node(name string) *testNode {
	n := g.nodes[name]
	if n == nil {
		n = &testNode{
			name:  name,
			edges: map[*testNode]float64{},
		}
		g.nodes[name] = n
	}
	return n
}

func (g *testGraph) ReadEdges(from Node, callback func(to Node, distance float64) (keepGoing bool)) {
	for other, dist := range from.(*testNode).edges {
		if !callback(other, dist) {
			return
		}
	}
}

func run(q *Query) map[string]*ShortestPath {
	ret := map[string]*ShortestPath{}
	q.Run(func(sp *ShortestPath) bool {
		name := sp.Node.Name()
		So(ret[name], ShouldBeNil)
		ret[name] = sp
		return true
	})
	return ret
}

func (g *testGraph) query(sources ...string) *Query {
	q := &Query{
		Sources:    make([]Node, len(sources)),
		EdgeReader: g,
	}
	for i, src := range sources {
		q.Sources[i] = g.node(src)
	}
	return q
}

type testNode struct {
	name  string
	edges map[*testNode]float64
}

func (n *testNode) Name() string {
	return n.name
}

func initGraph(edges ...testEdge) *testGraph {
	g := &testGraph{
		nodes: map[string]*testNode{},
	}

	for _, e := range edges {
		g.node(e.from).edges[g.node(e.to)] = e.distance
	}

	return g
}

type testEdge struct {
	from     string
	to       string
	distance float64
}

func TestQuery(t *testing.T) {
	t.Parallel()

	Convey(`Query`, t, func() {
		Convey(`Run`, func() {
			Convey(`Works`, func() {
				g := initGraph(
					testEdge{from: "//a", to: "//b/1", distance: 1},
					testEdge{from: "//a", to: "//b/2", distance: 2},
					testEdge{from: "//b/1", to: "//c", distance: 3},
					testEdge{from: "//b/2", to: "//c", distance: 3},
				)

				sps := run(g.query("//a"))
				So(sps, ShouldResemble, map[string]*ShortestPath{
					"//a": {
						Node:     g.node("//a"),
						Distance: 0,
					},
					"//b/1": {
						Prev:     sps["//a"],
						Node:     g.node("//b/1"),
						Distance: 1,
					},
					"//b/2": {
						Prev:     sps["//a"],
						Node:     g.node("//b/2"),
						Distance: 2,
					},
					"//c": {
						Prev:     sps["//b/1"],
						Node:     g.node("//c"),
						Distance: 4,
					},
				})
			})

			Convey(`MaxDistance`, func() {
				g := initGraph(
					testEdge{from: "//a", to: "//b/1", distance: 1},
					testEdge{from: "//a", to: "//b/2", distance: 2},
					testEdge{from: "//b/1", to: "//c", distance: 3},
					testEdge{from: "//b/2", to: "//c", distance: 3},
				)
				q := g.query("//a")
				q.MaxDistance = 3
				sps := run(q)
				So(sps, ShouldResemble, map[string]*ShortestPath{
					"//a": {
						Node:     g.node("//a"),
						Distance: 0,
					},
					"//b/1": {
						Prev:     sps["//a"],
						Node:     g.node("//b/1"),
						Distance: 1,
					},
					"//b/2": {
						Prev:     sps["//a"],
						Node:     g.node("//b/2"),
						Distance: 2,
					},
				})
			})

			Convey(`Unreachable`, func() {
				g := initGraph(
					testEdge{from: "//a", to: "//b", distance: 1},
					testEdge{from: "//c", to: "//d"},
				)

				sps := run(g.query("//a"))
				So(sps, ShouldResemble, map[string]*ShortestPath{
					"//a": {
						Node:     g.node("//a"),
						Distance: 0,
					},
					"//b": {
						Prev:     sps["//a"],
						Node:     g.node("//b"),
						Distance: 1,
					},
				})
			})

			Convey(`Visiting the same node multiple times`, func() {
				g := initGraph(
					testEdge{from: "//a", to: "//b", distance: 1},
					testEdge{from: "//a", to: "//c", distance: 10},
					testEdge{from: "//b", to: "//c", distance: 1},
				)
				g.query("//a") // asserts that each node is reported once
			})
			Convey(`Duplicate sources`, func() {
				g := initGraph(
					testEdge{from: "//a", to: "//b", distance: 1},
				)
				g.query("//a", "//a") // asserts that each node is reported once
			})
		})

		Convey(`ShortestPath`, func() {
			g := initGraph(
				testEdge{from: "//a", to: "//b/1", distance: 1},
				testEdge{from: "//a", to: "//b/2", distance: 2},
				testEdge{from: "//b/1", to: "//c", distance: 3},
				testEdge{from: "//b/2", to: "//c", distance: 3},
				testEdge{from: "//unreachable/1", to: "//unreachable/2"},
			)
			q := g.query("//a")

			Convey(`Works`, func() {
				sp := q.ShortestPath(g.node("//c"))
				So(sp, ShouldResemble, &ShortestPath{
					Node:     g.node("//c"),
					Distance: 4,
					Prev: &ShortestPath{
						Node:     g.node("//b/1"),
						Distance: 1,
						Prev: &ShortestPath{
							Node:     g.node("//a"),
							Distance: 0,
						},
					},
				})
				So(sp.Path(), ShouldResemble, []*ShortestPath{sp.Prev.Prev, sp.Prev, sp})
			})

			Convey(`Unreachable`, func() {
				So(q.ShortestPath(g.node("//unreachable/1")), ShouldBeNil)
			})
		})
	})
}
