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
			val, err := mmg.GetMeta(7, "hi")
			So(err, ShouldEqual, ErrMetaFieldUnset)
			So(val, ShouldBeNil)

			So(mmg.GetMetaDefault(7, "hi", "value"), ShouldEqual, "value")

			m := mmg.GetSingle(10)
			val, err = m.GetMeta("hi")
			So(err, ShouldEqual, ErrMetaFieldUnset)
			So(val, ShouldBeNil)
			So(m.GetMetaDefault("hi", "value"), ShouldEqual, "value")
		})

		Convey("stuff", func() {
			pmaps := []PropertyMap{{}, nil, {}}
			So(pmaps[0].SetMeta("hi", "thing"), ShouldBeNil)
			So(pmaps[2].SetMeta("key", 100), ShouldBeNil)
			mmg := NewMultiMetaGetter(pmaps)

			// oob is OK
			So(mmg.GetMetaDefault(7, "hi", "value"), ShouldEqual, "value")

			// nil is OK
			So(mmg.GetMetaDefault(1, "key", true), ShouldEqual, true)

			val, err := mmg.GetMeta(0, "hi")
			So(err, ShouldBeNil)
			So(val, ShouldEqual, "thing")

			So(mmg.GetMetaDefault(2, "key", 20), ShouldEqual, 100)
		})
	})
}
