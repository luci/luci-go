// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bf

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBitField(t *testing.T) {
	Convey("BitField", t, func() {
		bf := Make(2000)

		Convey("Should be sized right", func() {
			So(bf.Size(), ShouldEqual, 2000)
			So(len(bf.Data), ShouldEqual, 32)
		})

		Convey("Should be empty", func() {
			So(bf.All(false), ShouldBeTrue)
			So(bf.Data[0], ShouldEqual, 0)
			So(bf.CountSet(), ShouldEqual, 0)
			So(bf.IsSet(20), ShouldBeFalse)
			So(bf.IsSet(200001), ShouldBeFalse)

			Convey("and be unset", func() {
				for i := uint64(0); i < 20; i++ {
					So(bf.IsSet(i), ShouldBeFalse)
				}
			})
		})

		Convey("Boundary conditions are caught", func() {
			So(bf.Set(2000), ShouldNotBeNil)
			So(bf.Clear(2000), ShouldNotBeNil)
		})

		Convey("and setting [0, 1, 19, 197, 4]", func() {
			So(bf.Set(0), ShouldBeNil)
			So(bf.Set(1), ShouldBeNil)
			So(bf.Set(19), ShouldBeNil)
			So(bf.Set(197), ShouldBeNil)
			So(bf.Set(1999), ShouldBeNil)
			So(bf.Set(4), ShouldBeNil)

			Convey("should count correctly", func() {
				So(bf.CountSet(), ShouldEqual, 6)
			})

			Convey("should retrieve correctly", func() {
				So(bf.IsSet(2), ShouldBeFalse)
				So(bf.IsSet(18), ShouldBeFalse)

				So(bf.IsSet(0), ShouldBeTrue)
				So(bf.IsSet(1), ShouldBeTrue)
				So(bf.IsSet(4), ShouldBeTrue)
				So(bf.IsSet(19), ShouldBeTrue)
				So(bf.IsSet(197), ShouldBeTrue)
				So(bf.IsSet(1999), ShouldBeTrue)
			})

			Convey("should clear correctly", func() {
				So(bf.Clear(3), ShouldBeNil)
				So(bf.Clear(4), ShouldBeNil)
				So(bf.Clear(197), ShouldBeNil)

				So(bf.IsSet(2), ShouldBeFalse)
				So(bf.IsSet(3), ShouldBeFalse)
				So(bf.IsSet(18), ShouldBeFalse)

				So(bf.IsSet(4), ShouldBeFalse)
				So(bf.IsSet(197), ShouldBeFalse)

				So(bf.IsSet(0), ShouldBeTrue)
				So(bf.IsSet(1), ShouldBeTrue)
				So(bf.IsSet(19), ShouldBeTrue)

				So(bf.CountSet(), ShouldEqual, 4)
			})
		})
	})
}
