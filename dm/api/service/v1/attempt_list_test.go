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

package dm

import (
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAttemptListNormalize(t *testing.T) {
	t.Parallel()

	Convey("AttemptList.Normalize", t, func() {
		Convey("empty", func() {
			a := &AttemptList{}
			So(a.Normalize(), ShouldBeNil)
		})

		Convey("stuff", func() {
			a := &AttemptList{To: map[string]*AttemptList_Nums{}}
			a.To["quest"] = &AttemptList_Nums{Nums: []uint32{700, 2, 7, 90, 1, 1, 700}}
			a.To["other"] = &AttemptList_Nums{}
			a.To["other2"] = &AttemptList_Nums{Nums: []uint32{}}
			So(a.Normalize(), ShouldBeNil)

			So(a, ShouldResemble, &AttemptList{To: map[string]*AttemptList_Nums{
				"quest":  {Nums: []uint32{700, 90, 7, 2, 1}},
				"other":  {},
				"other2": {},
			}})
		})

		Convey("empty attempts / 0 attempts", func() {
			a := &AttemptList{To: map[string]*AttemptList_Nums{}}
			a.To["quest"] = nil
			a.To["other"] = &AttemptList_Nums{Nums: []uint32{0}}
			a.To["woot"] = &AttemptList_Nums{Nums: []uint32{}}
			So(a.Normalize(), ShouldBeNil)
			So(a, ShouldResemble, NewAttemptList(map[string][]uint32{
				"quest": nil,
				"other": nil,
				"woot":  nil,
			}))

			Convey("0 + other nums is error", func() {
				a.To["nerp"] = &AttemptList_Nums{Nums: []uint32{30, 7, 0}}
				So(a.Normalize(), ShouldErrLike, "contains 0 as well as other values")
			})
		})
	})

	Convey("AttemptList.AddAIDs", t, func() {
		list := &AttemptList{}
		list.AddAIDs(NewAttemptID("a", 1), NewAttemptID("b", 1), NewAttemptID("b", 2))
		So(list.To, ShouldResemble, map[string]*AttemptList_Nums{
			"a": {[]uint32{1}},
			"b": {[]uint32{1, 2}},
		})
	})
}
