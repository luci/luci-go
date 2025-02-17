// Copyright 2025 The LUCI Authors.
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

package indexset

import (
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSet(t *testing.T) {
	t.Parallel()

	ftt.Run("Test simple element updates", t, func(t *ftt.Test) {
		s := New(10)

		t.Run("Can add elements", func(t *ftt.Test) {
			assert.Loosely(t, s.Add(1), should.BeTrue)
			assert.Loosely(t, s.Add(1), should.BeFalse)
			assert.Loosely(t, s.Add(2), should.BeTrue)
			assert.Loosely(t, s.Len(), should.Equal(2))
			sl := s.ToSlice()
			sort.Slice(sl, func(i, j int) bool { return sl[i] < sl[j] })
			assert.Loosely(t, sl, should.Match([]uint32{1, 2}))

			t.Run("Can remove stuff", func(t *ftt.Test) {
				assert.Loosely(t, s.Remove(3), should.BeFalse)
				assert.Loosely(t, s.Remove(2), should.BeTrue)
				assert.Loosely(t, s.Remove(2), should.BeFalse)
				assert.Loosely(t, s.ToSlice(), should.Match([]uint32{1}))
			})
		})

		t.Run("Can add many elements at once", func(t *ftt.Test) {
			s.AddAll([]uint32{2, 1, 3})
			assert.Loosely(t, s.ToSortedSlice(), should.Match([]uint32{1, 2, 3}))

			t.Run("Can remove many elements at once", func(t *ftt.Test) {
				s.RemoveAll([]uint32{0, 2, 1})
				assert.Loosely(t, s.ToSlice(), should.Match([]uint32{3}))
			})
		})
	})

	ftt.Run("Can create with pre-set values", t, func(t *ftt.Test) {
		s := NewFromSlice(2, 1, 3, 0, 1)
		assert.Loosely(t, s.Len(), should.Equal(4))
		assert.Loosely(t, s.Has(2), should.BeTrue)
		assert.Loosely(t, s.HasAll(3, 1, 2, 0), should.BeTrue)
		assert.Loosely(t, s.HasAll(1, 4), should.BeFalse)

		t.Run("Can sort elements", func(t *ftt.Test) {
			assert.Loosely(t, s.ToSortedSlice(), should.Match([]uint32{0, 1, 2, 3}))
		})
	})

	ftt.Run("Can do set operations", t, func(t *ftt.Test) {
		s := NewFromSlice(0, 1, 2, 3, 4, 100)

		t.Run("Union", func(t *ftt.Test) {
			sl := s.Union(NewFromSlice(2, 9, 10)).ToSortedSlice()
			assert.Loosely(t, sl, should.Match([]uint32{0, 1, 2, 3, 4, 9, 10, 100}))
		})

		t.Run("Intersect", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				sl := s.Intersect(New(0)).ToSortedSlice()
				assert.Loosely(t, sl, should.BeEmpty)
			})

			t.Run("no overlap", func(t *ftt.Test) {
				sl := s.Intersect(NewFromSlice(5)).ToSortedSlice()
				assert.Loosely(t, sl, should.BeEmpty)
			})

			t.Run("some overlap", func(t *ftt.Test) {
				sl := s.Intersect(NewFromSlice(100, 3, 5, 101)).ToSortedSlice()
				assert.Loosely(t, sl, should.Match([]uint32{3, 100}))
			})

			t.Run("total overlap", func(t *ftt.Test) {
				sl := s.Intersect(NewFromSlice(1, 3, 100, 0, 2, 4)).ToSortedSlice()
				assert.Loosely(t, sl, should.Match([]uint32{0, 1, 2, 3, 4, 100}))
			})
		})

		t.Run("Difference", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				sl := s.Difference(New(0)).ToSortedSlice()
				assert.Loosely(t, sl, should.Match([]uint32{0, 1, 2, 3, 4, 100}))
			})

			t.Run("no overlap", func(t *ftt.Test) {
				sl := s.Difference(NewFromSlice(5, 99)).ToSortedSlice()
				assert.Loosely(t, sl, should.Match([]uint32{0, 1, 2, 3, 4, 100}))
			})

			t.Run("some overlap", func(t *ftt.Test) {
				sl := s.Difference(NewFromSlice(2, 4)).ToSortedSlice()
				assert.Loosely(t, sl, should.Match([]uint32{0, 1, 3, 100}))
			})

			t.Run("total overlap", func(t *ftt.Test) {
				sl := s.Difference(NewFromSlice(1, 3, 100, 0, 2, 4)).ToSortedSlice()
				assert.Loosely(t, sl, should.BeEmpty)
			})
		})

		t.Run("Contains", func(t *ftt.Test) {
			s1 := NewFromSlice(0, 1, 2)
			s2 := NewFromSlice(0, 1)
			assert.Loosely(t, s1.Contains(s2), should.BeTrue)
			assert.Loosely(t, s2.Contains(s1), should.BeFalse)
		})
	})
}
