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
	"bufio"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRead(t *testing.T) {
	t.Parallel()

	Convey(`Read`, t, func() {
		parseGraph := func(tokens ...string) *Graph {
			g := &Graph{}
			g.ensureInitialized()
			input := strings.Join(tokens, "\n") + "\n"
			r := &reader{
				r:        bufio.NewReader(strings.NewReader(input)),
				textMode: true,
			}
			err := r.readGraph(g)
			So(err, ShouldBeNil)
			return g
		}

		Convey(`Zero`, func() {
			g := parseGraph(
				"54", // header
				"0",  // version
				"",   // commit hash
				"0",  // number of root commits
				"0",  // number of root children
				"0",  // total number of edges
				"0",  // number of root edges
			)
			So(g.Commit, ShouldResemble, "")
			So(g.root, ShouldResemble, node{name: "//"})
		})

		Convey(`Two direct children`, func() {
			g := parseGraph(
				"54",       // header
				"0",        // version
				"deadbeef", // commit hash

				"0", // number of root commits
				"2", // number of root children

				"bar", // name of a root child
				"2",   // number of bar commits
				"0",   // number of bar children

				"foo", // name of a root child
				"1",   // number of foo commits
				"0",   // number of foo children

				"2", // total number of edges

				"0", // number of root edges

				"1", // number of bar edges
				"2", // index of the other node in edge #1
				"1", // number of the common commits between foo and bar

				"1", // number of foo edges
				"1", // index of the other node in edge #1
				"1", // number of the common commits between foo and bar
			)

			So(g.Commit, ShouldResemble, "deadbeef")
			So(g.root, ShouldResemble, node{
				name: "//",
				children: map[string]*node{
					"foo": {
						name:     "//foo",
						commits:  1,
						copyEdgesOnAppend: true,
						edges: []edge{{
							to:            g.root.children["bar"],
							commonCommits: 1,
						}},
					},
					"bar": {
						name:     "//bar",
						commits:  2,
						copyEdgesOnAppend: true,
						edges: []edge{{
							to:            g.root.children["foo"],
							commonCommits: 1,
						}},
					},
				},
			})
		})

		Convey(`Descendant name`, func() {
			g := parseGraph(
				"54",       // header
				"0",        // version
				"deadbeef", // commit hash

				"0", // number of root commits
				"1", // number of root children

				"dir", // name of a root child
				"0",   // number of dir commits
				"1",   // number of dir children

				"foo", // name of a dir child
				"1",   // number of foo commits
				"0",   // number of foo children

				"0", // total number of edges

				"0", // number of root edges
				"0", // number of dir edges
				"0", // number of foo edges
			)

			So(g.Commit, ShouldResemble, "deadbeef")
			So(g.root, ShouldResemble, node{
				name: "//",
				children: map[string]*node{
					"dir": {
						name: "//dir",
						children: map[string]*node{
							"foo": {
								name:    "//dir/foo",
								commits: 1,
							},
						},
					},
				},
			})
		})
	})
}
