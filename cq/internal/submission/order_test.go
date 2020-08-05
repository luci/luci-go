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

package submission

import (
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOptimize(t *testing.T) {
	t.Parallel()

	Convey("OptimizeNodes", t, func() {
		Convey("One", func() {
			So(optimize([]node{{key: "foo"}}), ShouldResemble, []node{{key: "foo"}})
		})

		Convey("Bogus deps", func() {
			nodeHard := node{
				key:      "withHardDep",
				outEdges: []edge{{destKey: "BogusHard", hard: true}},
			}
			nodeSoft := node{
				key:      "withSoftDep",
				outEdges: []edge{{destKey: "BogusSoft", hard: false}},
			}
			So(optimize([]node{nodeHard, nodeSoft}), ShouldResemble,
				[]node{nodeHard, nodeSoft})
		})

		Convey("Panic on duplicate keys", func() {
			So(func() { optimize([]node{{key: "foo"}, {key: "foo"}}) }, ShouldPanic)
		})

		Convey("Disjoint", func() {
			So(optimize([]node{{key: "foo"}, {key: "bar"}}), ShouldResemble,
				[]node{{key: "bar"}, {key: "foo"}})
		})

		Convey("Simple deps", func() {
			node1 := node{
				key:      "1",
				outEdges: []edge{{destKey: "2"}},
			}
			node2 := node{key: "2"}
			So(optimize([]node{node1, node2}), ShouldResemble, []node{node2, node1})

			Convey("With one hard dep", func() {
				node1.outEdges = []edge{{destKey: "2", hard: true}}
				So(optimize([]node{node1, node2}), ShouldResemble, []node{node2, node1})
			})
		})

		Convey("Break soft dependencies if cycle", func() {
			node1 := node{
				key:      "1",
				outEdges: []edge{{destKey: "2", hard: true}},
			}
			node2 := node{
				key:      "2",
				outEdges: []edge{{destKey: "1"}},
			}
			So(optimize([]node{node1, node2}), ShouldResemble, []node{node2, node1})
		})

		Convey("Panic if hard dependencies form a cycle", func() {
			node1 := node{
				key:      "1",
				outEdges: []edge{{destKey: "2", hard: true}},
			}
			node2 := node{
				key:      "2",
				outEdges: []edge{{destKey: "1", hard: true}},
			}
			So(func() { optimize([]node{node1, node2}) }, ShouldPanic)
		})

		Convey("Chain of 3", func() {
			node1 := node{
				key: "1",
				outEdges: []edge{
					{destKey: "2", hard: true},
					{destKey: "3", hard: true},
				},
			}
			node2 := node{
				key:      "2",
				outEdges: []edge{{destKey: "3", hard: true}},
			}
			node3 := node{
				key: "3",
			}
			So(optimize([]node{node1, node2, node3}), ShouldResemble,
				[]node{node3, node2, node1})

			Convey("Satisfy soft dep", func() {
				node1.outEdges = []edge{
					{destKey: "2"},
					{destKey: "3"},
				}
				node2.outEdges = []edge{{destKey: "3"}}

				So(optimize([]node{node1, node2, node3}), ShouldResemble,
					[]node{node3, node2, node1})
			})

			Convey("Ignore soft dep", func() {
				node3.outEdges = []edge{{destKey: "1"}}
				So(optimize([]node{node1, node2, node3}), ShouldResemble,
					[]node{node3, node2, node1})
				Convey("Input order doesn't matter", func() {
					So(optimize([]node{node3, node2, node1}), ShouldResemble,
						[]node{node3, node2, node1})
					So(optimize([]node{node2, node1, node3}), ShouldResemble,
						[]node{node3, node2, node1})
				})
			})
		})

		Convey("test pathologic", func() {
			n := 10
			nodes := genFullGraph(n)
			o := &optimizer{}
			So(o.optimize(nodes), ShouldResemble, nodes)
			So(len(o.removedEdges), ShouldEqual, (n-1)*(n-1))
		})

	})
}

// simulates a complete graph with N nodes s.t. Hard = True
// edges forming a linked chain {0 <- 1 <- 2 .. <- n-1}
// Total edges: n*(n-1), of which chain is length of (n-1)
func genFullGraph(numOfNodes int) []node {
	nodes := make([]node, numOfNodes)
	for i := range nodes {
		nodes[i] = node{
			key:      strconv.Itoa(i),
			outEdges: make([]edge, 0, numOfNodes-1),
		}
		for j := 0; j < numOfNodes; j++ {
			if i != j {
				nodes[i].outEdges = append(nodes[i].outEdges, edge{
					destKey: strconv.Itoa(j),
					hard:    j+1 == i,
				})
			}
		}
	}
	return nodes
}

func BenchmarkOptimize(b *testing.B) {
	nodes := genFullGraph(20)
	for i := 0; i < b.N; i++ {
		optimize(nodes)
	}
}
