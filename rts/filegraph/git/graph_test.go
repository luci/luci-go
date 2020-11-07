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
	"testing"

	"go.chromium.org/luci/rts/filegraph"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGraph(t *testing.T) {
	t.Parallel()

	Convey(`Graph`, t, func() {
		Convey(`Root of zero value`, func() {
			g := &Graph{}
			root := g.Node("//")
			So(root, ShouldNotBeNil)
			So(root.Name(), ShouldEqual, "//")
			So(root.IsFile(), ShouldBeFalse)
		})

		Convey(`node()`, func() {
			g := &Graph{
				root: node{
					children: map[string]*node{
						"dir": {
							children: map[string]*node{
								"foo": {},
							},
						},
					},
				},
			}

			Convey(`//`, func() {
				So(g.node("//"), ShouldEqual, &g.root)
			})

			Convey(`//dir`, func() {
				So(g.node("//dir"), ShouldEqual, g.root.children["dir"])
			})
			Convey(`//dir/foo`, func() {
				So(g.node("//dir/foo"), ShouldEqual, g.root.children["dir"].children["foo"])
			})
			Convey(`//dir/bar`, func() {
				So(g.node("//dir/bar"), ShouldBeNil)
			})
		})

		Convey(`ensureNode`, func() {
			g := &Graph{}
			Convey("//foo/bar", func() {
				bar := g.ensureNode("//foo/bar")
				So(bar, ShouldNotBeNil)
				So(bar.name, ShouldEqual, "//foo/bar")
				So(g.node("//foo/bar"), ShouldEqual, bar)

				foo := g.node("//foo")
				So(foo, ShouldNotBeNil)
				So(foo.name, ShouldEqual, "//foo")
				So(foo.children["bar"], ShouldEqual, bar)
			})

			Convey("already exists", func() {
				So(g.ensureNode("//foo/bar"), ShouldEqual, g.ensureNode("//foo/bar"))
			})

			Convey("//", func() {
				root := g.ensureNode("//")
				So(root, ShouldEqual, &g.root)
			})
		})

		Convey(`remove`, func() {
			g := &Graph{}
			Convey(`Single child`, func() {
				c := g.ensureNode("//a/b/c")
				So(g.remove("//a/b/c"), ShouldEqual, c)
				So(g.node("//a"), ShouldBeNil)
			})

			Convey(`Two children`, func() {
				c1 := g.ensureNode("//a/b/c1")
				c2 := g.ensureNode("//a/b/c2")
				So(g.remove("//a/b/c1"), ShouldEqual, c1)
				So(g.node("//a/b/c2"), ShouldEqual, c2)
			})

			Convey(`not found`, func() {
				g.ensureNode("//a/b/c")
				So(g.remove("//a/b/x"), ShouldBeNil)
			})
		})
		Convey(`moveFile`, func() {
			g := &Graph{}
			g.ensureInitialized()

			Convey(`move`, func() {
				oldFile := g.ensureNode("//a/b/c")
				oldFile.commits = 54
				newFile, err := g.moveFile("//a/b/c", "//x/y")
				So(err, ShouldBeNil)
				So(newFile, ShouldEqual, oldFile)
				So(newFile.commits, ShouldEqual, 54)
				So(g.node("//a/b"), ShouldBeNil)
				So(g.node("//a"), ShouldBeNil)
				So(g.node("//x/y"), ShouldEqual, newFile)
			})

			Convey(`remove`, func() {
				c := g.ensureNode("//a/b/c")
				x := g.ensureNode("//a/x")
				y := g.ensureNode("//a/y")
				c.edges = []edge{
					{to: x, commonCommits: 1},
					{to: y, commonCommits: 1},
				}
				x.edges = []edge{
					{to: c, commonCommits: 1},
					{to: y, commonCommits: 1},
				}
				y.edges = []edge{
					{to: c, commonCommits: 1},
					{to: x, commonCommits: 1},
				}

				removed, err := g.moveFile("//a/b/c", "")
				So(err, ShouldBeNil)
				So(removed, ShouldEqual, c)
				So(g.root, ShouldResemble, node{
					name: "//",
					children: map[string]*node{
						"a": {
							name: "//a",
							children: map[string]*node{
								"x": {
									name:  "//a/x",
									edges: []edge{{to: y, commonCommits: 1}},
								},
								"y": {
									name:  "//a/y",
									edges: []edge{{to: x, commonCommits: 1}},
								},
							},
						},
					},
				})
			})
		})
		Convey(`sortedChildKeys()`, func() {
			node := &node{
				children: map[string]*node{
					"foo": {},
					"bar": {},
				},
			}
			So(node.sortedChildKeys(), ShouldResemble, []string{"bar", "foo"})
		})

		Convey(`removeEdge`, func() {
			Convey(`Works`, func() {
				ns := make([]node, 4)
				ns[0].edges = []edge{
					{to: &ns[1]},
					{to: &ns[2]},
					{to: &ns[3]},
				}
				So(ns[0].removeEdge(&ns[2]), ShouldBeTrue)
				So(ns[0].edges, ShouldResemble, []edge{
					{to: &ns[1]},
					{to: &ns[3]},
				})
			})

			Convey(`not found`, func() {
				ns := make([]node, 4)
				ns[0].edges = []edge{
					{to: &ns[1]},
					{to: &ns[2]},
				}
				So(ns[0].removeEdge(&ns[3]), ShouldBeFalse)
			})
		})

		Convey(`Outgoing`, func() {
			bar := &node{commits: 2}
			foo := &node{
				commits: 2,
				edges:   []edge{{to: bar, commonCommits: 1}},
			}

			type outgoingEdge struct {
				other    filegraph.Node
				distance float64
			}
			var actual []outgoingEdge
			foo.Outgoing(func(other filegraph.Node, distance float64) bool {
				actual = append(actual, outgoingEdge{other: other, distance: distance})
				return true
			})
			So(actual, ShouldResemble, []outgoingEdge{{
				other:    bar,
				distance: 1,
			}})
		})

		Convey(`splitName`, func() {
			Convey("//foo/bar.cc", func() {
				So(splitName("//foo/bar.cc"), ShouldResemble, []string{"foo", "bar.cc"})
			})
			Convey("//", func() {
				So(splitName("//"), ShouldResemble, []string(nil))
			})
		})

		Convey(`baseName`, func() {
			Convey(`//foo/bar/qux`, func() {
				parent, base := baseName("//foo/bar/qux")
				So(parent, ShouldEqual, "//foo/bar")
				So(base, ShouldEqual, "qux")
			})
			Convey(`//foo`, func() {
				parent, base := baseName("//foo")
				So(parent, ShouldEqual, "//")
				So(base, ShouldEqual, "foo")
			})
		})
	})
}
