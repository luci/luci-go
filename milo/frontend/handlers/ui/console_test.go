// Copyright 2017 The LUCI Authors.
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

package ui

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/milo/internal/model"
)

type testBuilder struct {
	Builder  *BuilderRef
	Category []string
}

// Test helpers
func buildVerifyRoot(t *ftt.Test, name string, builders []testBuilder, expectChildren int) *Category {
	root := NewCategory(name)
	for _, builder := range builders {
		root.AddBuilder(builder.Category, builder.Builder)
	}
	assert.Loosely(t, len(root.Children()), should.Equal(expectChildren))
	assert.Loosely(t, root.Name, should.Equal(name))
	return root
}

func verifyCategory(t *ftt.Test, e ConsoleElement, expectChildren int, expectName string) *Category {
	cat := e.(*Category)
	assert.Loosely(t, len(cat.Children()), should.Equal(expectChildren))
	assert.Loosely(t, cat.Name, should.Equal(expectName))
	return cat
}

func TestCategory(t *testing.T) {
	ftt.Run("Category structure", t, func(t *ftt.Test) {
		// Test structures
		var emptycat []string
		cat1 := []string{"66__bbl"}
		cat2 := []string{"test.data"}
		deepcat := []string{"Hi", "Goodbye"}
		br1 := &BuilderRef{
			ID:        "test 1",
			ShortName: "t1",
			Build:     []*model.BuildSummary{},
		}
		br2 := &BuilderRef{
			ID:        "test 2",
			ShortName: "t2",
			Build:     []*model.BuildSummary{},
		}

		// Tests
		t.Run("Root category", func(t *ftt.Test) {
			buildVerifyRoot(t, "root", []testBuilder{}, 0)
		})

		t.Run("With builder", func(t *ftt.Test) {
			root := buildVerifyRoot(t, "_root_", []testBuilder{{br1, emptycat}}, 1)
			assert.Loosely(t, root.Children()[0].(*BuilderRef).ID, should.Equal(br1.ID))
		})

		t.Run("With nested categories", func(t *ftt.Test) {
			root := buildVerifyRoot(t, "o_o", []testBuilder{{br1, deepcat}}, 1)
			child1 := verifyCategory(t, root.Children()[0], 1, deepcat[0])
			child2 := verifyCategory(t, child1.Children()[0], 1, deepcat[1])
			assert.Loosely(t, child2.Children()[0].(*BuilderRef).ID, should.Equal(br1.ID))
		})

		t.Run("Multiple categories", func(t *ftt.Test) {
			root := buildVerifyRoot(t, "@_@", []testBuilder{
				{br1, cat1},
				{br2, cat2},
			}, 2)
			child1 := verifyCategory(t, root.Children()[0], 1, cat1[0])
			assert.Loosely(t, child1.Children()[0].(*BuilderRef).ID, should.Equal(br1.ID))
			child2 := verifyCategory(t, root.Children()[1], 1, cat2[0])
			assert.Loosely(t, child2.Children()[0].(*BuilderRef).ID, should.Equal(br2.ID))
		})

		t.Run("Reusing existing categories", func(t *ftt.Test) {
			root := buildVerifyRoot(t, "rut", []testBuilder{
				{br1, cat1},
				{br2, cat1},
			}, 1)
			child := verifyCategory(t, root.Children()[0], 2, cat1[0])
			assert.Loosely(t, child.Children()[0].(*BuilderRef).ID, should.Equal(br1.ID))
			assert.Loosely(t, child.Children()[1].(*BuilderRef).ID, should.Equal(br2.ID))
		})

		t.Run("Caches number of leaf nodes in a category", func(t *ftt.Test) {
			root := buildVerifyRoot(t, "rut", []testBuilder{{br1, cat1}}, 1)
			assert.Loosely(t, root.cachedNumLeafNodes, should.Equal(-1))
			assert.Loosely(t, root.Children()[0].(*Category).cachedNumLeafNodes, should.Equal(-1))
			assert.Loosely(t, root.NumLeafNodes(), should.Equal(1))
			assert.Loosely(t, root.cachedNumLeafNodes, should.Equal(1))
			assert.Loosely(t, root.Children()[0].(*Category).cachedNumLeafNodes, should.Equal(1))

			root.AddBuilder(cat1, br2) // this must invalidate cached values
			assert.Loosely(t, root.cachedNumLeafNodes, should.Equal(-1))
			assert.Loosely(t, root.Children()[0].(*Category).cachedNumLeafNodes, should.Equal(-1))
			assert.Loosely(t, root.NumLeafNodes(), should.Equal(2))
		})
	})
}
