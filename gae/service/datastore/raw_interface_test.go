// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
