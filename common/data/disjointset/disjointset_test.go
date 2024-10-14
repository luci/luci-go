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

package disjointset

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDisjointSet(t *testing.T) {
	t.Parallel()

	ftt.Run("DisjointSet works", t, func(t *ftt.Test) {
		d := New(5)
		indexes := make([]struct{}, 5)

		assert.Loosely(t, d.Merge(1, 1), should.BeFalse)
		assert.Loosely(t, d.Count(), should.Equal(5))

		for i := range indexes {
			assert.Loosely(t, d.RootOf(i), should.Equal(i))
			assert.Loosely(t, d.SizeOf(i), should.Equal(1))
			for j := range indexes {
				assert.Loosely(t, d.Disjoint(i, j), should.Equal(i != j))
			}
		}

		assert.Loosely(t, d.Merge(1, 4), should.BeTrue)
		assert.Loosely(t, d.Count(), should.Equal(4))
		assert.Loosely(t, d.SizeOf(1), should.Equal(2))

		assert.Loosely(t, d.Disjoint(1, 4), should.BeFalse)
		assert.Loosely(t, d.RootOf(4), should.Equal(1))
		assert.Loosely(t, d.RootOf(1), should.Equal(1))

		assert.Loosely(t, d.Merge(2, 3), should.BeTrue)
		assert.Loosely(t, d.SizeOf(2), should.Equal(2))

		assert.Loosely(t, d.Merge(1, 0), should.BeTrue)
		assert.Loosely(t, d.SizeOf(4), should.Equal(3))

		assert.Loosely(t, d.Merge(2, 1), should.BeTrue)
		assert.Loosely(t, d.SizeOf(3), should.Equal(5))

		for i := range indexes {
			for j := range indexes {
				assert.Loosely(t, d.Disjoint(i, j), should.BeFalse)
			}
		}
	})

	ftt.Run("DisjointSet SortedSets & String work", t, func(t *ftt.Test) {
		d := New(7)
		d.Merge(0, 4)
		d.Merge(0, 6)
		d.Merge(1, 3)
		d.Merge(2, 5)

		assert.Loosely(t, d.SortedSets(), should.Resemble([][]int{
			{0, 4, 6},
			{1, 3},
			{2, 5},
		}))

		assert.Loosely(t, d.String(), should.Match(`DisjointSet([
  [0, 4, 6]
  [1, 3]
  [2, 5]
])`))
	})
}
