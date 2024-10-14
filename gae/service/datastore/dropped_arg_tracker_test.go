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

package datastore

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDroppedArgTracker(t *testing.T) {
	t.Parallel()

	ftt.Run(`DroppedArgTracker`, t, func(t *ftt.Test) {
		t.Run(`nil`, func(t *ftt.Test) {
			var dat DroppedArgTracker
			dal := dat.mustCompress(200, func() {}, func(i, j int) {})

			assert.Loosely(t, dal.OriginalIndex(0), should.BeZero)
			assert.Loosely(t, dal.OriginalIndex(100), should.Equal(100))

			dat.MarkForRemoval(7, 10)
			assert.Loosely(t, dat, should.HaveLength(1))
		})

		t.Run(`DroppedArgLookup`, func(t *ftt.Test) {
			dal := DroppedArgLookup{
				{2, 3},
				{4, 7},
			}

			assert.Loosely(t, dal.OriginalIndex(0), should.BeZero)
			assert.Loosely(t, dal.OriginalIndex(1), should.Equal(1))
			assert.Loosely(t, dal.OriginalIndex(2), should.Equal(3))
			assert.Loosely(t, dal.OriginalIndex(3), should.Equal(4))
			assert.Loosely(t, dal.OriginalIndex(4), should.Equal(7))
			assert.Loosely(t, dal.OriginalIndex(10), should.Equal(13))
		})

		t.Run(`compress`, func(t *ftt.Test) {
			var dat DroppedArgTracker
			dat.MarkForRemoval(2, 11)
			dat.MarkForRemoval(5, 11)
			dat.MarkForRemoval(6, 11)

			dal := dat.mustCompress(11, func() {}, func(i, j int) {})
			assert.Loosely(t, dal, should.Resemble(DroppedArgLookup{
				{2, 3},
				{4, 7},
			}))
		})

		t.Run(`callbacks`, func(t *ftt.Test) {
			kc := MkKeyContext("app", "")
			mkKey := func(id string) *Key {
				return kc.MakeKey("kind", id)
			}

			input := []*Key{
				mkKey("a"),       // 0
				mkKey("whole"),   // 1
				mkKey("bunch"),   // 2
				mkKey("of"),      // 3
				mkKey("strings"), // 4
				mkKey("which"),   // 5
				mkKey("may"),     // 6
				mkKey("be"),      // 7
				mkKey("removed"), // 8
			}
			var dat DroppedArgTracker

			t.Run(`empty means no copy`, func(t *ftt.Test) {
				keys, _ := dat.DropKeys(input)
				assert.Loosely(t, keys, should.HaveLength(len(input)))
				// Make sure they're actually identical, and not a copy
				assert.Loosely(t, &keys[0], should.Equal(&input[0]))
			})

			t.Run(`can drop the last item`, func(t *ftt.Test) {
				dat.MarkForRemoval(len(input), len(input))
			})

			t.Run(`a couple dropped things`, func(t *ftt.Test) {
				// mark them out of order
				dat.MarkForRemoval(7, len(input))
				dat.MarkForRemoval(0, len(input))
				dat.MarkForRemoval(4, len(input))
				assert.Loosely(t, dat, should.HaveLength(3))

				// MarkForRemoval sorted them, woot.
				assert.Loosely(t, dat, should.Resemble(DroppedArgTracker{0, 4, 7}))
				reduced, dal := dat.DropKeys(input)
				assert.Loosely(t, dal, should.Resemble(DroppedArgLookup{
					{0, 1},
					{3, 5},
					{5, 8},
				}))

				assert.Loosely(t, reduced, should.Resemble([]*Key{
					mkKey("whole"), mkKey("bunch"), mkKey("of"),
					mkKey("which"), mkKey("may"), mkKey("removed"),
				}))

				for i, value := range reduced {
					assert.Loosely(t, input[dal.OriginalIndex(i)], should.Equal(value))
				}
			})
		})
	})
}
