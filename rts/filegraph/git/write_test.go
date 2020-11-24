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
	"bytes"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWrite(t *testing.T) {
	t.Parallel()

	Convey(`Write`, t, func() {
		buf := &bytes.Buffer{}
		w := writer{
			textMode: true,
			w:        buf,
		}

		test := func(g *Graph, expected ...string) {
			err := w.writeGraph(g)
			So(err, ShouldBeNil)
			actual := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
			So(actual, ShouldResemble, expected)
		}

		Convey(`Zero`, func() {
			test(&Graph{},
				"54", // header
				"0",  // version
				"",   // commit hash
				"0",  // number of root commits
				"0",  // number of root children
				"0",  // total number of edges
				"0",  // number of root edges
			)
		})

		Convey(`Two direct children`, func() {
			foo := &node{probSumDenominator: 1}
			bar := &node{probSumDenominator: 2}
			foo.edges = []edge{{to: bar, probSum: probOne}}
			bar.edges = []edge{{to: foo, probSum: probOne}}
			g := &Graph{
				Commit: "deadbeef",
				root: node{
					children: map[string]*node{
						"foo": foo,
						"bar": bar,
					},
				},
			}

			test(g,
				"54",       // header
				"0",        // version
				"deadbeef", // commit hash

				"0", // root's probSumDenominator
				"2", // number of root children

				"bar", // name of a root child
				"2",   // bar's probSumDenominator
				"0",   // number of bar children

				"foo", // name of a root child
				"1",   // foo's probSumDenominator
				"0",   // number of foo children

				"2", // total number of edges

				"0", // number of root edges

				"1",        // number of bar edges
				"2",        // index of foo
				"16777216", // probSum for bar->foo

				"1",        // number of foo edges
				"1",        // index of bar
				"16777216", // probSum for foo->bar
			)
		})
	})
}
