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
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadWrite(t *testing.T) {
	t.Parallel()

	Convey(`ReadWrite`, t, func() {
		test := func(g *Graph) {
			buf := &bytes.Buffer{}
			w := writer{w: buf}
			err := w.writeGraph(g)
			So(err, ShouldBeNil)
			graphBytes := buf.Bytes()

			r := reader{r: bufio.NewReader(buf)}
			var g2 Graph
			err = r.readGraph(&g2)
			So(err, ShouldBeNil)
			buf2 := &bytes.Buffer{}
			w2 := writer{w: buf2}
			err = w2.writeGraph(&g2)
			So(err, ShouldBeNil)

			So(graphBytes, ShouldResemble, buf2.Bytes())
		}

		Convey(`Zero`, func() {
			test(&Graph{})
		})

		Convey(`Two direct children`, func() {
			foo := &node{commits: 1}
			bar := &node{commits: 2}
			foo.edges = []edge{{to: bar, commonCommits: 1}}
			bar.edges = []edge{{to: foo, commonCommits: 1}}
			test(&Graph{
				Commit: "deadbeef",
				root: node{
					children: map[string]*node{
						"foo": foo,
						"bar": bar,
					},
				},
			})
		})
	})
}
