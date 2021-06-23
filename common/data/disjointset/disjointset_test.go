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

	. "github.com/smartystreets/goconvey/convey"
)

func TestDisjointSet(t *testing.T) {
	t.Parallel()

	Convey("DisjointSet works", t, func() {
		d := New(5)
		indexes := make([]struct{}, 5)

		So(d.Merge(1, 1), ShouldBeFalse)
		So(d.Count(), ShouldEqual, 5)

		for i := range indexes {
			So(d.RootOf(i), ShouldEqual, i)
			So(d.SizeOf(i), ShouldEqual, 1)
			for j := range indexes {
				So(d.Disjoint(i, j), ShouldEqual, i != j)
			}
		}

		So(d.Merge(1, 4), ShouldBeTrue)
		So(d.Count(), ShouldEqual, 4)
		So(d.SizeOf(1), ShouldEqual, 2)

		So(d.Disjoint(1, 4), ShouldBeFalse)
		So(d.RootOf(4), ShouldEqual, 1)
		So(d.RootOf(1), ShouldEqual, 1)

		So(d.Merge(2, 3), ShouldBeTrue)
		So(d.SizeOf(2), ShouldEqual, 2)

		So(d.Merge(1, 0), ShouldBeTrue)
		So(d.SizeOf(4), ShouldEqual, 3)

		So(d.Merge(2, 1), ShouldBeTrue)
		So(d.SizeOf(3), ShouldEqual, 5)

		for i := range indexes {
			for j := range indexes {
				So(d.Disjoint(i, j), ShouldBeFalse)
			}
		}
	})

	Convey("DisjointSet SortedSets & String work", t, func() {
		d := New(7)
		d.Merge(0, 4)
		d.Merge(0, 6)
		d.Merge(1, 3)
		d.Merge(2, 5)

		So(d.SortedSets(), ShouldResemble, [][]int{
			{0, 4, 6},
			{1, 3},
			{2, 5},
		})

		So(d.String(), ShouldResemble, `DisjointSet([
  [0, 4, 6]
  [1, 3]
  [2, 5]
])`)
	})
}
