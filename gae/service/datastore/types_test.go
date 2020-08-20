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

package datastore

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExistsResult(t *testing.T) {
	t.Parallel()

	Convey(`Testing ExistsResult`, t, func() {
		var er ExistsResult

		Convey(`With no elements`, func() {
			er.init()

			So(er.All(), ShouldBeTrue)
			So(er.Any(), ShouldBeFalse)
			So(er.List(), ShouldResemble, BoolList{})
		})

		Convey(`With single-tier elements`, func() {
			er.init(1, 1, 1, 1)

			So(er.All(), ShouldBeFalse)
			So(er.Any(), ShouldBeFalse)
			So(er.List(), ShouldResemble, BoolList{false, false, false, false})

			er.set(0, 0)
			er.set(2, 0)

			So(er.All(), ShouldBeFalse)
			So(er.Any(), ShouldBeTrue)
			So(er.List(), ShouldResemble, BoolList{true, false, true, false})
			So(er.List(0), ShouldResemble, BoolList{true})
			So(er.Get(0, 0), ShouldBeTrue)
			So(er.List(1), ShouldResemble, BoolList{false})
			So(er.Get(1, 0), ShouldBeFalse)
			So(er.List(2), ShouldResemble, BoolList{true})
			So(er.Get(2, 0), ShouldBeTrue)
			So(er.List(3), ShouldResemble, BoolList{false})
			So(er.Get(3, 0), ShouldBeFalse)
		})

		Convey(`With combined single- and multi-tier elements`, func() {
			er.init(1, 0, 3, 2)

			So(er.All(), ShouldBeFalse)
			So(er.Any(), ShouldBeFalse)
			So(er.List(), ShouldResemble, BoolList{false, false, false, false})

			// Set everything except (2, 1).
			er.set(0, 0)
			er.set(2, 0)
			er.set(2, 2)
			er.set(3, 0)
			er.set(3, 1)

			er.updateSlices()
			So(er.All(), ShouldBeFalse)
			So(er.Any(), ShouldBeTrue)
			So(er.List(), ShouldResemble, BoolList{true, true, false, true})
			So(er.List(0), ShouldResemble, BoolList{true})
			So(er.List(1), ShouldResemble, BoolList(nil))
			So(er.List(2), ShouldResemble, BoolList{true, false, true})
			So(er.Get(2, 0), ShouldBeTrue)
			So(er.Get(2, 1), ShouldBeFalse)
			So(er.List(3), ShouldResemble, BoolList{true, true})

			// Set the missing boolean.
			er.set(2, 1)
			er.updateSlices()
			So(er.All(), ShouldBeTrue)
		})

		Convey(`Zero-length slices are handled properly.`, func() {
			er.init(1, 0, 0, 1)

			er.updateSlices()
			So(er.List(), ShouldResemble, BoolList{false, true, true, false})
			So(er.All(), ShouldBeFalse)
			So(er.Any(), ShouldBeFalse)

			er.set(0, 0)
			er.updateSlices()
			So(er.List(), ShouldResemble, BoolList{true, true, true, false})
			So(er.All(), ShouldBeFalse)
			So(er.Any(), ShouldBeTrue)

			er.set(3, 0)
			er.updateSlices()
			So(er.List(), ShouldResemble, BoolList{true, true, true, true})
			So(er.All(), ShouldBeTrue)
			So(er.Any(), ShouldBeTrue)
		})
	})
}
