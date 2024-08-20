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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSubgraphOperations(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing addNode", t, func(t *ftt.Test) {
		subgraph := &Subgraph{
			Nodes:     []*SubgraphNode{},
			nodesToID: map[NodeKey]int32{},
		}

		t.Run("Testing adding group.", func(t *ftt.Test) {
			testGroup := "test-group"
			nodeID, placed := subgraph.addNode(Group, testGroup)
			assert.Loosely(t, nodeID, should.BeZero)
			assert.Loosely(t, placed, should.BeTrue)
		})

		t.Run("Testing adding user.", func(t *ftt.Test) {
			testUser := "user:m1@example.com"
			nodeID, placed := subgraph.addNode(Identity, testUser)
			assert.Loosely(t, nodeID, should.BeZero)
			assert.Loosely(t, placed, should.BeTrue)
		})

		t.Run("Testing adding glob.", func(t *ftt.Test) {
			testGlob := "user:*@example.com"
			nodeID, placed := subgraph.addNode(Glob, testGlob)
			assert.Loosely(t, nodeID, should.BeZero)
			assert.Loosely(t, placed, should.BeTrue)
		})

		t.Run("Testing adding same node.", func(t *ftt.Test) {
			testGroup := "test-group"
			nodeID, placed := subgraph.addNode(Group, testGroup)
			assert.Loosely(t, nodeID, should.BeZero)
			assert.Loosely(t, placed, should.BeTrue)
			nodeID, placed = subgraph.addNode(Group, testGroup)
			assert.Loosely(t, nodeID, should.BeZero)
			assert.Loosely(t, placed, should.BeFalse)
		})

		t.Run("Testing key for nodesToID.", func(t *ftt.Test) {
			testGroup := "test-group"
			testGlob := "user:*@example.com"
			testUser := "user:m1@example.com"
			subgraph.addNode(Group, testGroup)
			subgraph.addNode(Glob, testGlob)
			subgraph.addNode(Identity, testUser)

			expectedNodeMap := map[NodeKey]int32{
				{Group, testGroup}:   0,
				{Glob, testGlob}:     1,
				{Identity, testUser}: 2,
			}

			assert.Loosely(t, subgraph.nodesToID, should.Resemble(expectedNodeMap))
		})

	})

	ftt.Run("Testing addEdge", t, func(t *ftt.Test) {
		testGlob := "user:*@example.com"
		testGroup0 := "test-group-0"
		testUser := "user:m1@example.com"
		testGroup1 := "test-group-1"
		testGroup2 := "test-group-2"

		subgraph := &Subgraph{
			Nodes: []*SubgraphNode{
				{
					NodeKey: NodeKey{
						Kind:  Glob,
						Value: testGlob,
					},
				},
				{
					NodeKey: NodeKey{
						Kind:  Group,
						Value: testGroup0,
					},
				},
				{
					NodeKey: NodeKey{
						Kind:  Identity,
						Value: testUser,
					},
				},
				{
					NodeKey: NodeKey{
						Kind:  Group,
						Value: testGroup1,
					},
				},
				{
					NodeKey: NodeKey{
						Kind:  Group,
						Value: testGroup2,
					},
				},
			},
		}

		t.Run("Testing basic edge adding.", func(t *ftt.Test) {
			subgraph.addEdge(0, 1)
			subgraph.addEdge(2, 1)

			expectedSubgraph := &Subgraph{
				Nodes: []*SubgraphNode{
					{ // 0
						NodeKey: NodeKey{
							Kind:  Glob,
							Value: testGlob,
						},
						IncludedBy: []int32{1},
					},
					{ // 1
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup0,
						},
					},
					{ // 2
						NodeKey: NodeKey{
							Kind:  Identity,
							Value: testUser,
						},
						IncludedBy: []int32{1},
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
			}
			assert.Loosely(t, subgraph.Nodes, should.Resemble(expectedSubgraph.Nodes))
		})

		// Make sure that the order that of the edges stays consistent and is predictable.
		t.Run("Testing stability.", func(t *ftt.Test) {
			subgraph.addEdge(0, 4)
			subgraph.addEdge(0, 2)
			subgraph.addEdge(0, 3)
			subgraph.addEdge(2, 4)
			subgraph.addEdge(2, 3)
			subgraph.addEdge(2, 0)
			subgraph.addEdge(2, 1)
			expectedSubgraph := &Subgraph{
				Nodes: []*SubgraphNode{
					{ // 0
						NodeKey: NodeKey{
							Kind:  Glob,
							Value: testGlob,
						},
						IncludedBy: []int32{2, 3, 4},
					},
					{ // 1
						NodeKey: NodeKey{
							Kind:  Group,
							Value: testGroup0,
						},
					},
					{ // 2
						NodeKey: NodeKey{
							Kind:  Identity,
							Value: testUser,
						},
						IncludedBy: []int32{0, 1, 3, 4},
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
			}
			assert.Loosely(t, subgraph.Nodes, should.Resemble(expectedSubgraph.Nodes))
		})
	})
}
