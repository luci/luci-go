// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package serialize

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBinaryTools(t *testing.T) {
	t.Parallel()

	Convey("Test Join", t, func() {
		Convey("returns bytes with nil separator", func() {
			join := Join([]byte("hello"), []byte("world"))
			So(join, ShouldResemble, []byte("helloworld"))
		})
	})

	Convey("Test Invert", t, func() {
		Convey("returns nil for nil input", func() {
			inv := Invert(nil)
			So(inv, ShouldBeNil)
		})

		Convey("returns nil for empty input", func() {
			inv := Invert([]byte{})
			So(inv, ShouldBeNil)
		})

		Convey("returns byte slice of same length as input", func() {
			input := []byte("こんにちは, world")
			inv := Invert(input)
			So(len(input), ShouldEqual, len(inv))
		})

		Convey("returns byte slice with each byte inverted", func() {
			inv := Invert([]byte("foo"))
			So(inv, ShouldResemble, []byte{153, 144, 144})
		})
	})

	Convey("Test Increment", t, func() {
		Convey("returns empty slice and overflow true when input is nil", func() {
			incr, overflow := Increment(nil)
			So(incr, ShouldBeNil)
			So(overflow, ShouldBeTrue)
		})

		Convey("returns empty slice and overflow true when input is empty", func() {
			incr, overflow := Increment([]byte{})
			So(incr, ShouldBeNil)
			So(overflow, ShouldBeTrue)
		})

		Convey("handles overflow", func() {
			incr, overflow := Increment([]byte{0xFF, 0xFF})
			So(incr, ShouldResemble, []byte{0, 0})
			So(overflow, ShouldBeTrue)
		})

		Convey("increments with overflow false when there is no overflow", func() {
			incr, overflow := Increment([]byte{0xCA, 0xFF, 0xFF})
			So(incr, ShouldResemble, []byte{0xCB, 0, 0})
			So(overflow, ShouldBeFalse)
		})
	})
}
