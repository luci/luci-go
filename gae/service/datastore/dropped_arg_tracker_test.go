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

	. "github.com/smartystreets/goconvey/convey"
)

func TestDroppedArgTracker(t *testing.T) {
	t.Parallel()

	Convey(`DroppedArgTracker`, t, func() {
		Convey(`nil`, func() {
			var dat DroppedArgTracker
			dal := dat.mustCompress(200, func() {}, func(i, j int) {})

			So(dal.OriginalIndex(0), ShouldEqual, 0)
			So(dal.OriginalIndex(100), ShouldEqual, 100)

			dat.MarkForRemoval(7, 10)
			So(dat, ShouldHaveLength, 1)
		})

		Convey(`DroppedArgLookup`, func() {
			dal := DroppedArgLookup{
				{2, 3},
				{4, 7},
			}

			So(dal.OriginalIndex(0), ShouldEqual, 0)
			So(dal.OriginalIndex(1), ShouldEqual, 1)
			So(dal.OriginalIndex(2), ShouldEqual, 3)
			So(dal.OriginalIndex(3), ShouldEqual, 4)
			So(dal.OriginalIndex(4), ShouldEqual, 7)
			So(dal.OriginalIndex(10), ShouldEqual, 13)
		})

		Convey(`compress`, func() {
			var dat DroppedArgTracker
			dat.MarkForRemoval(2, 11)
			dat.MarkForRemoval(5, 11)
			dat.MarkForRemoval(6, 11)

			dal := dat.mustCompress(11, func() {}, func(i, j int) {})
			So(dal, ShouldResemble, DroppedArgLookup{
				{2, 3},
				{4, 7},
			})
		})

		Convey(`callbacks`, func() {
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

			Convey(`empty means no copy`, func() {
				keys, _ := dat.DropKeys(input)
				So(keys, ShouldHaveLength, len(input))
				// Make sure they're actually identical, and not a copy
				So(&keys[0], ShouldEqual, &input[0])
			})

			Convey(`can drop the last item`, func() {
				dat.MarkForRemoval(len(input), len(input))
			})

			Convey(`a couple dropped things`, func() {
				// mark them out of order
				dat.MarkForRemoval(7, len(input))
				dat.MarkForRemoval(0, len(input))
				dat.MarkForRemoval(4, len(input))
				So(dat, ShouldHaveLength, 3)

				// MarkForRemoval sorted them, woot.
				So(dat, ShouldResemble, DroppedArgTracker{0, 4, 7})
				reduced, dal := dat.DropKeys(input)
				So(dal, ShouldResemble, DroppedArgLookup{
					{0, 1},
					{3, 5},
					{5, 8},
				})

				So(reduced, ShouldResemble, []*Key{
					mkKey("whole"), mkKey("bunch"), mkKey("of"),
					mkKey("which"), mkKey("may"), mkKey("removed"),
				})

				for i, value := range reduced {
					So(input[dal.OriginalIndex(i)], ShouldEqual, value)
				}
			})
		})
	})
}
