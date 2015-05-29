// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBinutils(t *testing.T) {
	t.Parallel()

	Convey("Binary utilities", t, func() {
		b := &bytes.Buffer{}

		Convey("bytes", func() {
			t := []byte("this is a test")
			writeBytes(b, t)
			Convey("good", func() {
				r, err := readBytes(b)
				So(err, ShouldBeNil)
				So(r, ShouldResemble, t)
			})
			Convey("bad (truncated buffer)", func() {
				bs := b.Bytes()
				_, err := readBytes(bytes.NewBuffer(bs[:len(bs)-4]))
				So(err.Error(), ShouldContainSubstring, "readBytes: expected ")
			})
			Convey("bad (bad varint)", func() {
				_, err := readBytes(bytes.NewBuffer([]byte{0x8f}))
				So(err.Error(), ShouldContainSubstring, "EOF")
			})
		})

		Convey("strings", func() {
			t := "this is a test"
			writeString(b, t)
			Convey("good", func() {
				r, err := readString(b)
				So(err, ShouldBeNil)
				So(r, ShouldEqual, t)
			})
			Convey("bad (truncated buffer)", func() {
				bs := b.Bytes()
				_, err := readString(bytes.NewBuffer(bs[:len(bs)-4]))
				So(err.Error(), ShouldContainSubstring, "readBytes: expected ")
			})
		})
	})
}
