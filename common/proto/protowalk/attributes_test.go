// Copyright 2022 The LUCI Authors.
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

package protowalk

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRecurseAttr(t *testing.T) {
	t.Parallel()

	Convey(`RecurseAttr`, t, func() {
		Convey(`isMap`, func() {
			So(recurseNone.isMap(), ShouldBeFalse)
			So(recurseOne.isMap(), ShouldBeFalse)
			So(recurseRepeated.isMap(), ShouldBeFalse)

			So(recurseMapKind.isMap(), ShouldBeTrue)
			So(recurseMapBool.isMap(), ShouldBeTrue)
			So(recurseMapInt.isMap(), ShouldBeTrue)
			So(recurseMapUint.isMap(), ShouldBeTrue)
			So(recurseMapString.isMap(), ShouldBeTrue)
		})

		Convey(`set`, func() {
			r := recurseNone

			Convey(`OK`, func() {
				So(r.set(recurseOne), ShouldBeTrue)
				So(r, ShouldEqual, recurseOne)
				So(r.set(recurseOne), ShouldBeTrue)
				So(r, ShouldEqual, recurseOne)
			})

			Convey(`bad`, func() {
				So(r.set(recurseOne), ShouldBeTrue)
				So(r, ShouldEqual, recurseOne)
				So(func() { r.set(recurseRepeated) }, ShouldPanic)
			})
		})
	})

}

func TestProcessAttr(t *testing.T) {
	t.Parallel()

	Convey(`ProcessAttr`, t, func() {
		Convey(`applies`, func() {
			So(ProcessNever.applies(false), ShouldBeFalse)
			So(ProcessNever.applies(true), ShouldBeFalse)

			So(ProcessIfSet.applies(false), ShouldBeFalse)
			So(ProcessIfSet.applies(true), ShouldBeTrue)

			So(ProcessIfUnset.applies(false), ShouldBeTrue)
			So(ProcessIfUnset.applies(true), ShouldBeFalse)

			So(ProcessAlways.applies(false), ShouldBeTrue)
			So(ProcessAlways.applies(true), ShouldBeTrue)
		})

		Convey(`Valid`, func() {
			So(ProcessNever.Valid(), ShouldBeTrue)
			So(ProcessIfSet.Valid(), ShouldBeTrue)
			So(ProcessIfUnset.Valid(), ShouldBeTrue)
			So(ProcessAlways.Valid(), ShouldBeTrue)

			So(ProcessAttr(217).Valid(), ShouldBeFalse)
		})
	})

}
