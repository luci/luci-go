// Copyright 2015 The LUCI Authors.
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

package stringset

import (
	"reflect"
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSet(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Thread Unsafe Set", t, func(t *ftt.Test) {
		s := New(10)

		t.Run("Can add elements", func(t *ftt.Test) {
			assert.Loosely(t, s.Add("hello"), should.BeTrue)
			assert.Loosely(t, s.Add("hello"), should.BeFalse)
			assert.Loosely(t, s.Add("world"), should.BeTrue)
			assert.Loosely(t, s.Len(), should.Equal(2))
			sl := s.ToSlice()
			sort.Strings(sl)
			assert.Loosely(t, sl, should.Match([]string{"hello", "world"}))

			t.Run("Can remove stuff", func(t *ftt.Test) {
				assert.Loosely(t, s.Del("foo"), should.BeFalse)
				assert.Loosely(t, s.Del("world"), should.BeTrue)
				assert.Loosely(t, s.Del("world"), should.BeFalse)
				assert.Loosely(t, s.ToSlice(), should.Match([]string{"hello"}))
			})

			t.Run("Can peek them", func(t *ftt.Test) {
				str, found := s.Peek()
				assert.Loosely(t, found, should.BeTrue)
				assert.Loosely(t, sl, should.Contain(str))

				_, found = New(10).Peek()
				assert.Loosely(t, found, should.BeFalse)
			})

			t.Run("Can pop them", func(t *ftt.Test) {
				var newList []string
				str, found := s.Pop()
				assert.Loosely(t, found, should.BeTrue)
				newList = append(newList, str)

				str, found = s.Pop()
				assert.Loosely(t, found, should.BeTrue)
				newList = append(newList, str)

				_, found = s.Pop()
				assert.Loosely(t, found, should.BeFalse)
				assert.Loosely(t, s.Len(), should.BeZero)

				sort.Strings(newList)

				assert.Loosely(t, newList, should.Match(sl))
			})

			t.Run("Can iterate", func(t *ftt.Test) {
				s.Iter(func(val string) bool {
					assert.Loosely(t, sl, should.Contain(val))
					return true
				})
			})

			t.Run("Can stop iteration early", func(t *ftt.Test) {
				foundOne := false
				s.Iter(func(val string) bool {
					assert.Loosely(t, foundOne, should.BeFalse)
					foundOne = true
					assert.Loosely(t, sl, should.Contain(val))
					return false
				})
			})

			t.Run("Can dup them", func(t *ftt.Test) {
				dup := s.Dup()
				assert.Loosely(t, dup, should.Match(s))
				assert.Loosely(t, reflect.ValueOf(dup).Pointer(), should.NotEqual(reflect.ValueOf(s).Pointer()))
				dup.Add("panwaffles") // the best of both!
				assert.Loosely(t, dup, should.NotResemble(s))
			})
		})
	})

	ftt.Run("Can create with pre-set values", t, func(t *ftt.Test) {
		s := NewFromSlice("hi", "there", "person", "hi")
		assert.Loosely(t, s.Len(), should.Equal(3))
		assert.Loosely(t, s.Has("hi"), should.BeTrue)
		assert.Loosely(t, s.HasAll("hi", "person"), should.BeTrue)
		assert.Loosely(t, s.HasAll("hi", "bye"), should.BeFalse)
		sl := s.ToSlice()
		sort.Strings(sl)
		assert.Loosely(t, sl, should.Match([]string{"hi", "person", "there"}))
	})

	ftt.Run("Can do set operations", t, func(t *ftt.Test) {
		s := NewFromSlice("a", "b", "c", "d", "e", "f", "z")

		t.Run("Union", func(t *ftt.Test) {
			sl := s.Union(NewFromSlice("b", "k", "g")).ToSlice()
			sort.Strings(sl)
			assert.Loosely(t, sl, should.Match([]string{
				"a", "b", "c", "d", "e", "f", "g", "k", "z"}))
		})

		t.Run("Intersect", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				sl := s.Intersect(New(0)).ToSlice()
				sort.Strings(sl)
				assert.Loosely(t, sl, should.BeEmpty)
			})

			t.Run("no overlap", func(t *ftt.Test) {
				sl := s.Intersect(NewFromSlice("beef")).ToSlice()
				sort.Strings(sl)
				assert.Loosely(t, sl, should.BeEmpty)
			})

			t.Run("some overlap", func(t *ftt.Test) {
				sl := s.Intersect(NewFromSlice("c", "k", "z", "g")).ToSlice()
				sort.Strings(sl)
				assert.Loosely(t, sl, should.Match([]string{"c", "z"}))
			})

			t.Run("total overlap", func(t *ftt.Test) {
				sl := s.Intersect(NewFromSlice("a", "b", "c", "d", "e", "f", "z")).ToSlice()
				sort.Strings(sl)
				assert.Loosely(t, sl, should.Match([]string{"a", "b", "c", "d", "e", "f", "z"}))
			})
		})

		t.Run("Difference", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				sl := s.Difference(New(0)).ToSlice()
				sort.Strings(sl)
				assert.Loosely(t, sl, should.Match([]string{"a", "b", "c", "d", "e", "f", "z"}))
			})

			t.Run("no overlap", func(t *ftt.Test) {
				sl := s.Difference(NewFromSlice("beef")).ToSlice()
				sort.Strings(sl)
				assert.Loosely(t, sl, should.Match([]string{"a", "b", "c", "d", "e", "f", "z"}))
			})

			t.Run("some overlap", func(t *ftt.Test) {
				sl := s.Difference(NewFromSlice("c", "k", "z", "g")).ToSlice()
				sort.Strings(sl)
				assert.Loosely(t, sl, should.Match([]string{"a", "b", "d", "e", "f"}))
			})

			t.Run("total overlap", func(t *ftt.Test) {
				sl := s.Difference(NewFromSlice("a", "b", "c", "d", "e", "f", "z")).ToSlice()
				sort.Strings(sl)
				assert.Loosely(t, sl, should.BeEmpty)
			})
		})

		t.Run("Contains", func(t *ftt.Test) {
			s1 := NewFromSlice("a", "b", "c")
			s2 := NewFromSlice("a", "b")
			assert.Loosely(t, s1.Contains(s2), should.BeTrue)
			assert.Loosely(t, s2.Contains(s1), should.BeFalse)
		})
	})
}
