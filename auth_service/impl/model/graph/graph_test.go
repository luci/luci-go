// Copyright 2021 The LUCI Authors.
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

package graph

import (
	"strings"
	"testing"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/auth_service/impl/model"

	. "github.com/smartystreets/goconvey/convey"
)

////////////////////////////////////////////////////////////////////////////////////////
// Helper functions for tests.

func testAuthGroup(name string, items ...string) *model.AuthGroup {
	var globs, identities, nested []string
	for _, item := range items {
		if strings.Contains(item, "*") {
			globs = append(globs, item)
		} else if strings.Contains(item, ":") {
			identities = append(identities, item)
		} else {
			nested = append(nested, item)
		}
	}

	return &model.AuthGroup{
		ID:      name,
		Members: identities,
		Globs:   globs,
		Nested:  nested,
		Owners:  "owners-" + name,
	}
}

////////////////////////////////////////////////////////////////////////////////////////

func TestGraphBuilding(t *testing.T) {
	t.Parallel()

	Convey("Testing basic Graph Building.", t, func() {

		authGroups := []*model.AuthGroup{
			testAuthGroup("group-0", "user:m1@example.com", "user:*@example.com"),
			testAuthGroup("group-1", "user:m1@example.com", "user:m2@example.com"),
			testAuthGroup("group-2", "user:*@example.com"),
		}

		actualGraph := NewGraph(authGroups)

		expectedGraph := &Graph{
			groups: map[string]*groupNode{
				"group-0": {
					group: authGroups[0],
				},
				"group-1": {
					group: authGroups[1],
				},
				"group-2": {
					group: authGroups[2],
				},
			},
			globs: []identity.Glob{
				identity.Glob("user:*@example.com"),
			},
			membersIndex: map[identity.Identity][]string{
				identity.Identity("user:m1@example.com"): {"group-0", "group-1"},
				identity.Identity("user:m2@example.com"): {"group-1"},
			},
			globsIndex: map[identity.Glob][]string{
				identity.Glob("user:*@example.com"): {"group-0", "group-2"},
			},
		}

		So(actualGraph, ShouldResemble, expectedGraph)
	})

	Convey("Testing group nesting.", t, func() {
		authGroups := []*model.AuthGroup{
			testAuthGroup("group-0"),
			testAuthGroup("group-1", "group-0"),
			testAuthGroup("group-2", "group-1"),
		}

		actualGraph := NewGraph(authGroups)

		So(actualGraph.groups["group-0"].included[0].group, ShouldResemble, authGroups[1])
		So(actualGraph.groups["group-1"].included[0].group, ShouldResemble, authGroups[2])
		So(actualGraph.groups["group-1"].includes[0].group, ShouldResemble, authGroups[0])
		So(actualGraph.groups["group-2"].includes[0].group, ShouldResemble, authGroups[1])
	})
}

func TestGetRelevantSubgraph(t *testing.T) {
	t.Parallel()

	Convey("Testing GetRelevantSubgraph", t, func() {
		testGroup0 := "group-0"
		testGroup1 := "group-1"
		testGroup2 := "group-2"
		testUser0 := "user:m0@example.com"
		testUser1 := "user:m1@example.com"
		testGlob := "user:*@example.com"

		authGroups := []*model.AuthGroup{
			testAuthGroup(testGroup0, testUser0, testGlob),
			testAuthGroup(testGroup1, testUser0, testUser1, testGroup0),
			testAuthGroup(testGroup2, testGlob),
		}

		graph := NewGraph(authGroups)

		Convey("Testing Group Principal.", func() {
			principal := NodeKey{Group, testGroup1}

			subgraph, err := graph.GetRelevantSubgraph(principal)
			So(err, ShouldBeNil)

			expectedSubgraph := &Subgraph{
				Nodes: []*SubgraphNode{
					{ // 0
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup1,
						},
					},
				},
				nodesToID: map[NodeKey]int32{
					{Group, testGroup1}: 0,
				},
			}

			So(subgraph, ShouldResemble, expectedSubgraph)
		})

		Convey("Testing Identity Principal.", func() {
			principal := NodeKey{Identity, testUser0}

			subgraph, err := graph.GetRelevantSubgraph(principal)
			So(err, ShouldBeNil)

			expectedSubgraph := &Subgraph{
				Nodes: []*SubgraphNode{
					{ // 0
						NodeKey: NodeKey{
							Kind:  Identity,
							Value: testUser0,
						},
						IncludedBy: []int32{1, 2, 3},
					},
					{ // 1
						NodeKey: NodeKey{
							Kind:  Glob,
							Value: testGlob,
						},
						IncludedBy: []int32{2, 4},
					},
					{ // 2
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup0,
						},
						IncludedBy: []int32{3},
					},
					{ // 3
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup1,
						},
					},
					{ // 4
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup2,
						},
					},
				},
				nodesToID: map[NodeKey]int32{
					{Identity, testUser0}: 0,
					{Glob, testGlob}:      1,
					{Group, testGroup0}:   2,
					{Group, testGroup1}:   3,
					{Group, testGroup2}:   4,
				},
			}

			So(subgraph, ShouldResemble, expectedSubgraph)
		})

		Convey("Testing Glob principal.", func() {
			principal := NodeKey{Glob, testGlob}

			subgraph, err := graph.GetRelevantSubgraph(principal)
			So(err, ShouldBeNil)

			expectedSubgraph := &Subgraph{
				Nodes: []*SubgraphNode{
					{ // 0
						NodeKey: NodeKey{
							Kind:  Glob,
							Value: testGlob,
						},
						IncludedBy: []int32{1, 3},
					},
					{ // 1
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup0,
						},
						IncludedBy: []int32{2},
					},
					{ // 2
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup1,
						},
					},
					{ // 3
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup2,
						},
					},
				},
				nodesToID: map[NodeKey]int32{
					{Glob, testGlob}:    0,
					{Group, testGroup0}: 1,
					{Group, testGroup1}: 2,
					{Group, testGroup2}: 3,
				},
			}

			So(subgraph, ShouldResemble, expectedSubgraph)
		})

		Convey("Testing Stability", func() {
			principal := NodeKey{Identity, testUser0}
			testGlob1 := "*ser:m0@example.com"
			testGlob2 := "user:m0@*"
			testGlob3 := "*"
			testGlob4 := "user:m0@example.*"

			authGroups2 := []*model.AuthGroup{
				testAuthGroup(testGroup0, testGlob),
				testAuthGroup(testGroup1, testGlob1),
				testAuthGroup(testGroup2, testGlob2),
				testAuthGroup("group-3", testGlob3),
				testAuthGroup("group-4", testGlob4),
			}
			graph2 := NewGraph(authGroups2)

			subgraph, err := graph2.GetRelevantSubgraph(principal)
			So(err, ShouldBeNil)

			expectedSubgraph := &Subgraph{
				Nodes: []*SubgraphNode{
					{ // 0
						NodeKey: NodeKey{
							Kind:  Identity,
							Value: testUser0,
						},
						IncludedBy: []int32{1, 3, 5},
					},
					{ // 1
						NodeKey: NodeKey{
							Kind:  Glob,
							Value: testGlob,
						},
						IncludedBy: []int32{2},
					},
					{ // 2
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup0,
						},
					},
					{ // 3
						NodeKey: NodeKey{
							Kind:  Glob,
							Value: testGlob2,
						},
						IncludedBy: []int32{4},
					},
					{ // 4
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup2,
						},
					},
					{ // 5
						NodeKey: NodeKey{
							Kind:  Glob,
							Value: testGlob4,
						},
						IncludedBy: []int32{6},
					},
					{ // 6
						NodeKey: NodeKey{
							Kind:  Group,
							Value: "group-4",
						},
					},
				},
				nodesToID: map[NodeKey]int32{
					{Identity, testUser0}: 0,
					{Glob, testGlob}:      1,
					{Group, testGroup0}:   2,
					{Glob, testGlob2}:     3,
					{Group, testGroup2}:   4,
					{Glob, testGlob4}:     5,
					{Group, "group-4"}:    6,
				},
			}

			So(subgraph, ShouldResemble, expectedSubgraph)
		})
	})
}
