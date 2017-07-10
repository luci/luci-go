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

func TestMultiMetaGetter(t *testing.T) {
	t.Parallel()

	Convey("Test MultiMetaGetter", t, func() {
		Convey("nil", func() {
			mmg := NewMultiMetaGetter(nil)
			val, ok := mmg.GetMeta(7, "hi")
			So(ok, ShouldBeFalse)
			So(val, ShouldBeNil)

			So(GetMetaDefault(mmg.GetSingle(7), "hi", "value"), ShouldEqual, "value")

			m := mmg.GetSingle(10)
			val, ok = m.GetMeta("hi")
			So(ok, ShouldBeFalse)
			So(val, ShouldBeNil)

			So(GetMetaDefault(m, "hi", "value"), ShouldEqual, "value")
		})

		Convey("stuff", func() {
			pmaps := []PropertyMap{{}, nil, {}}
			So(pmaps[0].SetMeta("hi", "thing"), ShouldBeTrue)
			So(pmaps[2].SetMeta("key", 100), ShouldBeTrue)
			mmg := NewMultiMetaGetter(pmaps)

			// oob is OK
			So(GetMetaDefault(mmg.GetSingle(7), "hi", "value"), ShouldEqual, "value")

			// nil is OK
			So(GetMetaDefault(mmg.GetSingle(1), "key", true), ShouldEqual, true)

			val, ok := mmg.GetMeta(0, "hi")
			So(ok, ShouldBeTrue)
			So(val, ShouldEqual, "thing")

			So(GetMetaDefault(mmg.GetSingle(2), "key", 20), ShouldEqual, 100)
		})
	})
}
