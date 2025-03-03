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

package copyonwrite

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type el struct {
	id, val int
}

func (el el) Equal(other el) bool {
	return el.id == other.id && el.val == other.val
}

// evenCubedOddDeleted deletes `el`s with odd IDs and sets val to id^3 for even
// ones.
func evenCubedOddDeleted(v any) any {
	in := v.(*el)
	if in.id&1 == 1 {
		return Deletion
	}
	c := in.id * in.id * in.id
	if in.val == c {
		return in
	}
	return &el{in.id, c}
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	mustNoop := func(in Slice, m Modifier, add Slice) {
		out, u := Update(in, m, add)
		assert.Loosely(t, u, should.BeFalse)
		assert.That(t, out, should.Match(in))
	}

	ftt.Run("Update noops", t, func(t *ftt.Test) {
		t.Run("empty", func(t *ftt.Test) {
			mustNoop(nil, nil, nil)
			mustNoop(elSlice{}, nil, nil)
			mustNoop(nil, evenCubedOddDeleted, nil)
			mustNoop(nil, evenCubedOddDeleted, elSlice{})
		})
		t.Run("no changes", func(t *ftt.Test) {
			mustNoop(elSlice{{0, 0}, {2, 8}, {4, 64}}, nil, nil)
			mustNoop(elSlice{{0, 0}, {2, 8}, {4, 64}}, evenCubedOddDeleted, nil)
			mustNoop(elSlice{{0, 0}, {2, 8}, {4, 64}}, evenCubedOddDeleted, elSlice{})
		})
	})

	ftt.Run("Update works on Slice", t, func(t *ftt.Test) {
		t.Run("deletes", func(t *ftt.Test) {
			res, u := Update(elSlice{{1, 0}, {4, 64}, {3, 0}, {0, 0}}, evenCubedOddDeleted, nil)
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSlice{{4, 64}, {0, 0}}))
		})
		t.Run("modifies", func(t *ftt.Test) {
			res, u := Update(elSlice{{2, 8}, {4, 0}}, evenCubedOddDeleted, nil)
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSlice{{2, 8}, {4, 64}}))
		})
		t.Run("modifies and deletes", func(t *ftt.Test) {
			res, u := Update(elSlice{{1, 0}, {2, 8}, {3, 0}, {4, 0}}, evenCubedOddDeleted, nil)
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSlice{{2, 8}, {4, 64}}))
		})
		t.Run("creates on empty", func(t *ftt.Test) {
			res, u := Update(nil, nil, elSlice{{6, 0}, {5, 0}})
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSlice{{6, 0}, {5, 0}}))
		})
		t.Run("creates", func(t *ftt.Test) {
			res, u := Update(elSlice{{1, 0}}, nil, elSlice{{6, 0}, {5, 0}})
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSlice{{6, 0}, {5, 0}, {1, 0}}))
		})
		t.Run("creates, modifies and deletes", func(t *ftt.Test) {
			res, u := Update(elSlice{{1, 0}, {2, 8}, {3, 0}, {4, 0}}, evenCubedOddDeleted, elSlice{{5, 25}, {0, 0}})
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSlice{{5, 25}, {0, 0}, {2, 8}, {4, 64}}))
		})
	})

	ftt.Run("Update works on SortedSlice", t, func(t *ftt.Test) {
		t.Run("panics if toAdd is not sorted", func(t *ftt.Test) {
			assert.Loosely(t, func() { Update(elSortedSlice{}, nil, elSlice{{3, 8}}) },
				should.PanicLikeString("Different types for in and toAdd slices"))
		})
		t.Run("creates sorted", func(t *ftt.Test) {
			res, u := Update(elSortedSlice{}, nil, elSortedSlice{{3, 8}, {1, 8}, {4, 1}, {2, 0}})
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSortedSlice{{1, 8}, {2, 0}, {3, 8}, {4, 1}}))
		})
		t.Run("modifies and deletes", func(t *ftt.Test) {
			res, u := Update(elSortedSlice{{1, 0}, {2, 8}, {3, 0}, {4, 0}}, evenCubedOddDeleted, nil)
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSortedSlice{{2, 8}, {4, 64}}))
		})
		t.Run("deletes everything", func(t *ftt.Test) {
			res, u := Update(elSortedSlice{{1, 0}, {3, 0}}, evenCubedOddDeleted, nil)
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSortedSlice{}))
		})
		t.Run("creates, modifies and deletes", func(t *ftt.Test) {
			in := elSortedSlice{{1, 0}, {2, 8}, {3, 0}, {4, 0}, {6, 1}, {7, 3}}
			res, u := Update(in, evenCubedOddDeleted, elSortedSlice{{10, 100}, {0, 0}, {5, 25}})
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSortedSlice{{0, 0}, {2, 8}, {4, 64}, {5, 25}, {6, 216}, {10, 100}}))

			res, u = Update(in, evenCubedOddDeleted, elSortedSlice{{3, 3}})
			assert.Loosely(t, u, should.BeTrue)
			assert.Loosely(t, res, should.Match(elSortedSlice{{2, 8}, {3, 3}, {4, 64}, {6, 216}}))
		})
	})
}

type elSlice []*el

var _ Slice = elSlice(nil)

func (e elSlice) Len() int {
	return len(e)
}

func (e elSlice) At(index int) any {
	return e[index]
}

func (e elSlice) Append(v any) Slice {
	return append(e, v.(*el))
}

func (e elSlice) CloneShallow(length int, capacity int) Slice {
	r := make(elSlice, length, capacity)
	copy(r, e[:length])
	return r
}

type elSortedSlice []*el

var _ SortedSlice = elSortedSlice(nil)

func (e elSortedSlice) Len() int {
	return len(e)
}

func (e elSortedSlice) At(index int) any {
	return e[index]
}

func (e elSortedSlice) Append(v any) Slice {
	return append(e, v.(*el))
}

func (e elSortedSlice) CloneShallow(length int, capacity int) Slice {
	r := make(elSortedSlice, length, capacity)
	copy(r, e[:length])
	return r
}

func (e elSortedSlice) Less(i int, j int) bool {
	return e[i].id < e[j].id
}

func (e elSortedSlice) Swap(i int, j int) {
	e[i], e[j] = e[j], e[i]
}

func (_ elSortedSlice) LessElements(a any, b any) bool {
	return a.(*el).id < b.(*el).id
}
