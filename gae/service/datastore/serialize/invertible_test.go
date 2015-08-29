// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package serialize

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInvertible(t *testing.T) {
	t.Parallel()

	Convey("Test InvertibleByteBuffer", t, func() {
		inv := Invertible(&bytes.Buffer{})

		Convey("normal writing", func() {
			Convey("Write", func() {
				n, err := inv.Write([]byte("hello"))
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 5)
				So(inv.String(), ShouldEqual, "hello")
			})
			Convey("WriteString", func() {
				n, err := inv.WriteString("hello")
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 5)
				So(inv.String(), ShouldEqual, "hello")
			})
			Convey("WriteByte", func() {
				for i := byte('a'); i < 'f'; i++ {
					err := inv.WriteByte(i)
					So(err, ShouldBeNil)
				}
				So(inv.String(), ShouldEqual, "abcde")

				Convey("ReadByte", func() {
					for i := 0; i < 5; i++ {
						b, err := inv.ReadByte()
						So(err, ShouldBeNil)
						So(b, ShouldEqual, byte('a')+byte(i))
					}
				})
			})
		})
		Convey("inverted writing", func() {
			inv.SetInvert(true)
			Convey("Write", func() {
				n, err := inv.Write([]byte("hello"))
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 5)
				So(inv.String(), ShouldEqual, "\x97\x9a\x93\x93\x90")
			})
			Convey("WriteString", func() {
				n, err := inv.WriteString("hello")
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 5)
				So(inv.String(), ShouldEqual, "\x97\x9a\x93\x93\x90")
			})
			Convey("WriteByte", func() {
				for i := byte('a'); i < 'f'; i++ {
					err := inv.WriteByte(i)
					So(err, ShouldBeNil)
				}
				So(inv.String(), ShouldEqual, "\x9e\x9d\x9c\x9b\x9a")

				Convey("ReadByte", func() {
					for i := 0; i < 5; i++ {
						b, err := inv.ReadByte()
						So(err, ShouldBeNil)
						So(b, ShouldEqual, byte('a')+byte(i)) // inverted back to normal
					}
				})
			})
		})
		Convey("Toggleable", func() {
			inv.SetInvert(true)
			n, err := inv.Write([]byte("hello"))
			So(err, ShouldBeNil)
			inv.SetInvert(false)
			n, err = inv.Write([]byte("hello"))
			So(n, ShouldEqual, 5)
			So(inv.String(), ShouldEqual, "\x97\x9a\x93\x93\x90hello")
		})
	})
}
