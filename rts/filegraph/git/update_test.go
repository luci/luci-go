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

	. "github.com/smartystreets/goconvey/convey"
)

func TestApply(t *testing.T) {
	t.Parallel()

	Convey(`apply`, t, func() {
		g := &Graph{}
		g.ensureInitialized()

		Convey(`Empty change`, func() {
			err := g.apply(nil)
			So(err, ShouldBeNil)
			So(g.root, ShouldResemble, node{name: "//"})
		})

		Convey(`Add one file`, func() {
			err := g.apply([]fileChange{
				{Path: "a", Status: 'A'},
			})
			So(err, ShouldBeNil)
			So(g.root, ShouldResemble, node{
				name: "//",
				children: map[string]*node{
					"a": {
						name:    "//a",
						commits: 1,
					},
				},
			})
		})

		Convey(`Add two files`, func() {
			err := g.apply([]fileChange{
				{Path: "a", Status: 'A'},
				{Path: "b", Status: 'A'},
			})
			So(err, ShouldBeNil)
			So(g.root, ShouldResemble, node{
				name: "//",
				children: map[string]*node{
					"a": {
						name:    "//a",
						commits: 1,
						edges:   []edge{{to: g.node("//b"), commonCommits: 1}},
					},
					"b": {
						name:    "//b",
						commits: 1,
						edges:   []edge{{to: g.node("//a"), commonCommits: 1}},
					},
				},
			})

			Convey(`Add two more`, func() {
				err := g.apply([]fileChange{
					{Path: "b", Status: 'A'},
					{Path: "c/d", Status: 'A'},
				})
				So(err, ShouldBeNil)
				So(g.root, ShouldResemble, node{
					name: "//",
					children: map[string]*node{
						"a": {
							name:    "//a",
							commits: 1,
							edges:   []edge{{to: g.node("//b"), commonCommits: 1}},
						},
						"b": {
							name:    "//b",
							commits: 2,
							edges: []edge{
								{to: g.node("//a"), commonCommits: 1},
								{to: g.node("//c/d"), commonCommits: 1},
							},
						},
						"c": {
							name: "//c",
							children: map[string]*node{
								"d": {
									name:    "//c/d",
									commits: 1,
									edges:   []edge{{to: g.node("//b"), commonCommits: 1}},
								},
							},
						},
					},
				})
			})

			Convey(`Modify them again`, func() {
				err := g.apply([]fileChange{
					{Path: "a", Status: 'M'},
					{Path: "b", Status: 'M'},
				})
				So(err, ShouldBeNil)
				So(g.root, ShouldResemble, node{
					name: "//",
					children: map[string]*node{
						"a": {
							name:    "//a",
							commits: 2,
							edges:   []edge{{to: g.node("//b"), commonCommits: 2}},
						},
						"b": {
							name:    "//b",
							commits: 2,
							edges:   []edge{{to: g.node("//a"), commonCommits: 2}},
						},
					},
				})

			})

			Convey(`Modify one and add another`, func() {
				err := g.apply([]fileChange{
					{Path: "b", Status: 'M'},
					{Path: "c", Status: 'M'},
				})
				So(err, ShouldBeNil)
				So(g.root, ShouldResemble, node{
					name: "//",
					children: map[string]*node{
						"a": {
							name:    "//a",
							commits: 1,
							edges:   []edge{{to: g.node("//b"), commonCommits: 1}},
						},
						"b": {
							name:    "//b",
							commits: 2,
							edges: []edge{
								{to: g.node("//a"), commonCommits: 1},
								{to: g.node("//c"), commonCommits: 1},
							},
						},
						"c": {
							name:    "//c",
							commits: 1,
							edges:   []edge{{to: g.node("//b"), commonCommits: 1}},
						},
					},
				})
			})

			Convey(`Rename one`, func() {
				err := g.apply([]fileChange{
					{Path: "b", Path2: "c", Status: 'R'},
				})
				So(err, ShouldBeNil)
				So(g.root, ShouldResemble, node{
					name: "//",
					children: map[string]*node{
						"a": {
							name:    "//a",
							commits: 1,
							edges:   []edge{{to: g.node("//b"), commonCommits: 1}},
						},
						"b": {
							name:    "//b",
							commits: 1,
							edges: []edge{
								{to: g.node("//a"), commonCommits: 1},
								{to: g.node("//c")},
							},
						},
						"c": {
							name:    "//c",
							commits: 1,
							edges:   []edge{{to: g.node("//b")}},
						},
					},
				})
			})

			Convey(`Remove one`, func() {
				err := g.apply([]fileChange{
					{Path: "b", Status: 'D'},
				})
				So(err, ShouldBeNil)
				So(g.root, ShouldResemble, node{
					name: "//",
					children: map[string]*node{
						"a": {
							name:    "//a",
							commits: 1,
							edges:   []edge{{to: g.node("//b"), commonCommits: 1}},
						},
						"b": {
							name:    "//b",
							commits: 1,
							edges:   []edge{{to: g.node("//a"), commonCommits: 1}},
						},
					},
				})
			})
		})
	})
}
