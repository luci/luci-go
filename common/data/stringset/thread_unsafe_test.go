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
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestThreadUnsafeSet(t *testing.T) {
	t.Parallel()

	Convey("Test Thread Unsafe Set", t, func() {
		s := New(10)

		Convey("Can add elements", func() {
			So(s.Add("hello"), ShouldBeTrue)
			So(s.Add("hello"), ShouldBeFalse)
			So(s.Add("world"), ShouldBeTrue)
			So(s.Len(), ShouldEqual, 2)
			sl := s.ToSlice()
			sort.Strings(sl)
			So(sl, ShouldResemble, []string{"hello", "world"})

			Convey("Can remove stuff", func() {
				So(s.Del("foo"), ShouldBeFalse)
				So(s.Del("world"), ShouldBeTrue)
				So(s.Del("world"), ShouldBeFalse)
				So(s.ToSlice(), ShouldResemble, []string{"hello"})
			})

			Convey("Can peek them", func() {
				str, found := s.Peek()
				So(found, ShouldBeTrue)
				So(sl, ShouldContain, str)

				_, found = New(10).Peek()
				So(found, ShouldBeFalse)
			})

			Convey("Can pop them", func() {
				var newList []string
				str, found := s.Pop()
				So(found, ShouldBeTrue)
				newList = append(newList, str)

				str, found = s.Pop()
				So(found, ShouldBeTrue)
				newList = append(newList, str)

				_, found = s.Pop()
				So(found, ShouldBeFalse)
				So(s.Len(), ShouldEqual, 0)

				sort.Strings(newList)

				So(newList, ShouldResemble, sl)
			})

			Convey("Can iterate", func() {
				s.Iter(func(val string) bool {
					So(sl, ShouldContain, val)
					return true
				})
			})

			Convey("Can stop iteration early", func() {
				foundOne := false
				s.Iter(func(val string) bool {
					So(foundOne, ShouldBeFalse)
					foundOne = true
					So(sl, ShouldContain, val)
					return false
				})
			})

			Convey("Can dup them", func() {
				dup := s.Dup()
				So(dup, ShouldResemble, s)
				So(dup, ShouldNotEqual, s)
				dup.Add("panwaffles") // the best of both!
				So(dup, ShouldNotResemble, s)
			})
		})
	})

	Convey("Can create with pre-set values", t, func() {
		s := NewFromSlice("hi", "there", "person", "hi")
		So(s.Len(), ShouldEqual, 3)
		So(s.Has("hi"), ShouldBeTrue)
		sl := s.ToSlice()
		sort.Strings(sl)
		So(sl, ShouldResemble, []string{"hi", "person", "there"})
	})

	Convey("Can do set operations", t, func() {
		s := NewFromSlice("a", "b", "c", "d", "e", "f", "z")

		Convey("Union", func() {
			sl := s.Union(NewFromSlice("b", "k", "g")).ToSlice()
			sort.Strings(sl)
			So(sl, ShouldResemble, []string{
				"a", "b", "c", "d", "e", "f", "g", "k", "z"})
		})

		Convey("Intersect", func() {
			Convey("empty", func() {
				sl := s.Intersect(New(0)).ToSlice()
				sort.Strings(sl)
				So(sl, ShouldBeEmpty)
			})

			Convey("no overlap", func() {
				sl := s.Intersect(NewFromSlice("beef")).ToSlice()
				sort.Strings(sl)
				So(sl, ShouldBeEmpty)
			})

			Convey("some overlap", func() {
				sl := s.Intersect(NewFromSlice("c", "k", "z", "g")).ToSlice()
				sort.Strings(sl)
				So(sl, ShouldResemble, []string{"c", "z"})
			})

			Convey("total overlap", func() {
				sl := s.Intersect(NewFromSlice("a", "b", "c", "d", "e", "f", "z")).ToSlice()
				sort.Strings(sl)
				So(sl, ShouldResemble, []string{"a", "b", "c", "d", "e", "f", "z"})
			})
		})

		Convey("Difference", func() {
			Convey("empty", func() {
				sl := s.Difference(New(0)).ToSlice()
				sort.Strings(sl)
				So(sl, ShouldResemble, []string{"a", "b", "c", "d", "e", "f", "z"})
			})

			Convey("no overlap", func() {
				sl := s.Difference(NewFromSlice("beef")).ToSlice()
				sort.Strings(sl)
				So(sl, ShouldResemble, []string{"a", "b", "c", "d", "e", "f", "z"})
			})

			Convey("some overlap", func() {
				sl := s.Difference(NewFromSlice("c", "k", "z", "g")).ToSlice()
				sort.Strings(sl)
				So(sl, ShouldResemble, []string{"a", "b", "d", "e", "f"})
			})

			Convey("total overlap", func() {
				sl := s.Difference(NewFromSlice("a", "b", "c", "d", "e", "f", "z")).ToSlice()
				sort.Strings(sl)
				So(sl, ShouldBeEmpty)
			})
		})
	})
}
