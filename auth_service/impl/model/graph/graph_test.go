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
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/auth_service/impl/model"
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

	graphComp := cmp.AllowUnexported(Graph{}, GroupNode{})

	ftt.Run("Testing basic Graph Building.", t, func(t *ftt.Test) {
		authGroups := []*model.AuthGroup{
			testAuthGroup("group-0", "user:m1@example.com", "user:*@example.com"),
			testAuthGroup("group-1", "user:m1@example.com", "user:m2@example.com"),
			testAuthGroup("group-2", "user:*@example.com"),
		}

		graphableGroups := make([]model.GraphableGroup, len(authGroups))
		for i, group := range authGroups {
			graphableGroups[i] = model.GraphableGroup(group)
		}

		actualGraph := NewGraph(graphableGroups)

		expectedGraph := &Graph{
			groups: map[string]*GroupNode{
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
			membersIndex: map[identity.NormalizedIdentity][]string{
				identity.NewNormalizedIdentity("user:m1@example.com"): {"group-0", "group-1"},
				identity.NewNormalizedIdentity("user:m2@example.com"): {"group-1"},
			},
			globsIndex: map[identity.Glob][]string{
				identity.Glob("user:*@example.com"): {"group-0", "group-2"},
			},
		}

		assert.Loosely(t, actualGraph, should.Match(expectedGraph, graphComp))
	})

	ftt.Run("Testing group nesting.", t, func(t *ftt.Test) {
		authGroups := []*model.AuthGroup{
			testAuthGroup("group-0"),
			testAuthGroup("group-1", "group-0"),
			testAuthGroup("group-2", "group-1"),
		}

		graphableGroups := make([]model.GraphableGroup, len(authGroups))
		for i, group := range authGroups {
			graphableGroups[i] = model.GraphableGroup(group)
		}

		actualGraph := NewGraph(graphableGroups)

		assert.Loosely(t, actualGraph.groups["group-0"].included[0].group,
			should.Match(authGroups[1], graphComp))
		assert.Loosely(t, actualGraph.groups["group-1"].included[0].group,
			should.Match(authGroups[2], graphComp))
		assert.Loosely(t, actualGraph.groups["group-1"].includes[0].group,
			should.Match(authGroups[0], graphComp))
		assert.Loosely(t, actualGraph.groups["group-2"].includes[0].group,
			should.Match(authGroups[1], graphComp))
	})
}

func TestGetExpandedGroup(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing GetExpandedGroup", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:someone@example.com",
		})

		testGroup0 := "group-0"
		testGroup1 := "group-1"
		testGroup2 := "group-2"
		testUser0 := "user:m0@example.com"
		testUser1 := "user:m1@example.com"
		testGlob := "user:*@example.com"
		testGoogleGroupA := "google/test-group-a"
		testSysGroupA := "sys/test-group-a"

		authGroups := []*model.AuthGroup{
			testAuthGroup(testGroup0, testUser0, testGlob),
			testAuthGroup(testGroup1, testUser0, testUser1, testGroup0),
			testAuthGroup(testGroup2, testGroup1, testGroup0),
			testAuthGroup(testGoogleGroupA, testUser0, testUser1),
			testAuthGroup(testSysGroupA, testUser1, testGoogleGroupA),
		}

		graphableGroups := make([]model.GraphableGroup, len(authGroups))
		for i, group := range authGroups {
			graphableGroups[i] = model.GraphableGroup(group)
		}

		graph := NewGraph(graphableGroups)

		t.Run("unknown group should return error", func(t *ftt.Test) {
			_, err := graph.GetExpandedGroup(ctx, "unknown-group", false, nil)
			assert.Loosely(t, errors.Is(err, ErrNoSuchGroup), should.BeTrue)
		})

		t.Run("group with no nesting works", func(t *ftt.Test) {
			expanded, err := graph.GetExpandedGroup(ctx, testGroup0, false, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expanded, should.Match(&ExpandedGroup{
				Name:     testGroup0,
				Owners:   "owners-" + testGroup0,
				Members:  stringset.NewFromSlice(testUser0),
				Globs:    stringset.NewFromSlice(testGlob),
				Nested:   stringset.New(0),
				Redacted: stringset.New(0),
			}))
		})

		t.Run("group with nested group works", func(t *ftt.Test) {
			expanded, err := graph.GetExpandedGroup(ctx, testGroup1, false, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expanded, should.Match(&ExpandedGroup{
				Name:     testGroup1,
				Owners:   "owners-" + testGroup1,
				Members:  stringset.NewFromSlice(testUser0, testUser1),
				Globs:    stringset.NewFromSlice(testGlob),
				Nested:   stringset.NewFromSlice(testGroup0),
				Redacted: stringset.New(0),
			}))
		})

		t.Run("subgroup nested twice", func(t *ftt.Test) {
			expanded, err := graph.GetExpandedGroup(ctx, testGroup2, false, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expanded, should.Match(&ExpandedGroup{
				Name:     testGroup2,
				Owners:   "owners-" + testGroup2,
				Members:  stringset.NewFromSlice(testUser0, testUser1),
				Globs:    stringset.NewFromSlice(testGlob),
				Nested:   stringset.NewFromSlice(testGroup0, testGroup1),
				Redacted: stringset.New(0),
			}))
		})

		t.Run("member filter works", func(t *ftt.Test) {
			expanded, err := graph.GetExpandedGroup(ctx, testGoogleGroupA, false, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expanded, should.Match(&ExpandedGroup{
				Name:     testGoogleGroupA,
				Owners:   "owners-" + testGoogleGroupA,
				Members:  stringset.New(0),
				Globs:    stringset.New(0),
				Nested:   stringset.New(0),
				Redacted: stringset.NewFromSlice(testUser0, testUser1),
			}))
		})

		t.Run("member filter can be skipped", func(t *ftt.Test) {
			expanded, err := graph.GetExpandedGroup(ctx, testGoogleGroupA, true, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expanded, should.Match(&ExpandedGroup{
				Name:     testGoogleGroupA,
				Owners:   "owners-" + testGoogleGroupA,
				Members:  stringset.NewFromSlice(testUser0, testUser1),
				Globs:    stringset.New(0),
				Nested:   stringset.New(0),
				Redacted: stringset.New(0),
			}))
		})

		t.Run("privileged user can see members", func(t *ftt.Test) {
			adminCTX := auth.WithState(context.Background(), &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{model.AdminGroup},
			})
			expanded, err := graph.GetExpandedGroup(adminCTX, testGoogleGroupA, false, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expanded, should.Match(&ExpandedGroup{
				Name:     testGoogleGroupA,
				Owners:   "owners-" + testGoogleGroupA,
				Members:  stringset.NewFromSlice(testUser0, testUser1),
				Globs:    stringset.New(0),
				Nested:   stringset.New(0),
				Redacted: stringset.New(0),
			}))
		})
	})
}

func TestGetRelevantSubgraph(t *testing.T) {
	t.Parallel()

	subgraphComp := cmp.AllowUnexported(Subgraph{})

	ftt.Run("Testing GetRelevantSubgraph", t, func(t *ftt.Test) {
		testGroup0 := "group-0"
		testGroup1 := "group-1"
		testGroup2 := "group-2"
		testUser0 := "user:m0@example.com"
		testUser0MatchingGlob := "user:M0@example.com"
		testUser0MixedCasing := "user:M0@ExAmPlE.CoM"
		testUser1 := "user:m1@example.com"
		testGlob := "user:*@example.com"

		authGroups := []*model.AuthGroup{
			testAuthGroup(testGroup0, testUser0, testGlob),
			testAuthGroup(testGroup1, testUser0, testUser1, testGroup0),
			testAuthGroup(testGroup2, testGlob),
		}

		graphableGroups := make([]model.GraphableGroup, len(authGroups))
		for i, group := range authGroups {
			graphableGroups[i] = model.GraphableGroup(group)
		}

		graph := NewGraph(graphableGroups)

		t.Run("Testing Group Principal.", func(t *ftt.Test) {
			principal := NodeKey{Group, testGroup1}

			subgraph, err := graph.GetRelevantSubgraph(principal)
			assert.Loosely(t, err, should.BeNil)

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

			assert.Loosely(t, subgraph, should.Match(expectedSubgraph, subgraphComp))
		})

		t.Run("Testing Identity Principal.", func(t *ftt.Test) {
			principal := NodeKey{Identity, testUser0}

			subgraph, err := graph.GetRelevantSubgraph(principal)
			assert.Loosely(t, err, should.BeNil)

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

			assert.Loosely(t, subgraph, should.Match(expectedSubgraph, subgraphComp))

			t.Run("equivalent Identity principal", func(t *ftt.Test) {
				principal := NodeKey{Identity, testUser0MatchingGlob}

				subgraph, err := graph.GetRelevantSubgraph(principal)
				assert.Loosely(t, err, should.BeNil)

				expectedSubgraph := &Subgraph{
					Nodes: []*SubgraphNode{
						{ // 0
							NodeKey: NodeKey{
								Kind:  Identity,
								Value: testUser0MatchingGlob,
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
						{Identity, testUser0MatchingGlob}: 0,
						{Glob, testGlob}:                  1,
						{Group, testGroup0}:               2,
						{Group, testGroup1}:               3,
						{Group, testGroup2}:               4,
					},
				}

				assert.Loosely(t, subgraph, should.Match(expectedSubgraph, subgraphComp))
			})
		})

		t.Run("Identity principal respects glob case", func(t *ftt.Test) {
			principal := NodeKey{Identity, testUser0MixedCasing}
			subgraph, err := graph.GetRelevantSubgraph(principal)
			assert.Loosely(t, err, should.BeNil)

			expectedSubgraph := &Subgraph{
				Nodes: []*SubgraphNode{
					{ // 0
						NodeKey: NodeKey{
							Kind:  Identity,
							Value: testUser0MixedCasing,
						},
						IncludedBy: []int32{1, 2},
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
				},
				nodesToID: map[NodeKey]int32{
					{Identity, testUser0MixedCasing}: 0,
					{Group, testGroup0}:              1,
					{Group, testGroup1}:              2,
				},
			}

			assert.Loosely(t, subgraph, should.Match(expectedSubgraph, subgraphComp))
		})

		t.Run("Testing Glob principal.", func(t *ftt.Test) {
			principal := NodeKey{Glob, testGlob}

			subgraph, err := graph.GetRelevantSubgraph(principal)
			assert.Loosely(t, err, should.BeNil)

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

			assert.Loosely(t, subgraph, should.Match(expectedSubgraph, subgraphComp))
		})

		t.Run("Testing Stability", func(t *ftt.Test) {
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
			graphableGroups2 := make([]model.GraphableGroup, len(authGroups2))
			for i, group := range authGroups2 {
				graphableGroups2[i] = model.GraphableGroup(group)
			}

			graph2 := NewGraph(graphableGroups2)

			subgraph, err := graph2.GetRelevantSubgraph(principal)
			assert.Loosely(t, err, should.BeNil)

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

			assert.Loosely(t, subgraph, should.Match(expectedSubgraph, subgraphComp))
		})
	})
}
