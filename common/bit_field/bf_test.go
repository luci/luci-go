// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bf

import (
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBitField(t *testing.T) {
	Convey("BitField", t, func() {
		bf := Make(2000)

		Convey("Should be sized right", func() {
			So(bf.Size(), ShouldEqual, 2000)
			So(len(bf.data), ShouldEqual, 250)
		})

		Convey("Should be empty", func() {
			So(bf.All(false), ShouldBeTrue)
			So(bf.data[0], ShouldEqual, 0)
			So(bf.CountSet(), ShouldEqual, 0)
			So(bf.IsSet(20), ShouldBeFalse)
			So(bf.IsSet(200001), ShouldBeFalse)

			Convey("and be unset", func() {
				for i := uint32(0); i < 20; i++ {
					So(bf.IsSet(i), ShouldBeFalse)
				}
			})
		})

		Convey("Boundary conditions are caught", func() {
			So(func() { bf.Set(2000) }, ShouldPanicLike, "cannot set bit 2000")
			So(func() { bf.Clear(2000) }, ShouldPanicLike, "cannot clear bit 2000")
		})

		Convey("and setting [0, 1, 19, 197, 4]", func() {
			bf.Set(0)
			bf.Set(1)
			bf.Set(19)
			bf.Set(197)
			bf.Set(1999)
			bf.Set(4)

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
				bf.Clear(3)
				bf.Clear(4)
				bf.Clear(197)

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

		Convey("Can interact with datastore", func() {
			Convey("encodes to a []byte", func() {
				p, err := bf.ToProperty()
				So(err, ShouldBeNil)

				bval := make([]byte, 252)
				// varint encoding of 2000
				bval[0] = 208
				bval[1] = 15
				So(p.Value(), ShouldResembleV, bval)

				Convey("decodes as well", func() {
					nbf := BitField{}
					So(nbf.FromProperty(p), ShouldBeNil)
					So(nbf, ShouldResembleV, bf)
				})
			})

			Convey("zero-length BitField has small representation", func() {
				bf = Make(0)
				p, err := bf.ToProperty()
				So(err, ShouldBeNil)
				So(p.Value(), ShouldResembleV, []byte{0})

				Convey("decodes as well", func() {
					nbf := BitField{}
					So(nbf.FromProperty(p), ShouldBeNil)
					So(nbf, ShouldResembleV, bf)
				})
			})

			Convey("setting bits round-trips", func() {
				bf.Set(0)
				bf.Set(1)
				bf.Set(19)
				bf.Set(197)
				bf.Set(1999)
				bf.Set(4)

				p, err := bf.ToProperty()
				So(err, ShouldBeNil)

				bval := make([]byte, 252)
				// varint encoding of 2000
				bval[0] = 208
				bval[1] = 15
				// various bits set
				bval[2] = 19    // 0 and 1 and 4
				bval[4] = 8     // 19
				bval[26] = 32   // 197
				bval[251] = 128 // 1999
				So(p.Value(), ShouldResembleV, bval)

				nbf := BitField{}
				So(nbf.FromProperty(p), ShouldBeNil)
				So(nbf, ShouldResembleV, bf)

				So(nbf.IsSet(2), ShouldBeFalse)
				So(nbf.IsSet(18), ShouldBeFalse)

				So(nbf.IsSet(0), ShouldBeTrue)
				So(nbf.IsSet(1), ShouldBeTrue)
				So(nbf.IsSet(4), ShouldBeTrue)
				So(nbf.IsSet(19), ShouldBeTrue)
				So(nbf.IsSet(197), ShouldBeTrue)
				So(nbf.IsSet(1999), ShouldBeTrue)
			})

			Convey("empty sets have canonical representation", func() {
				bf = Make(0)
				p, err := bf.ToProperty()
				So(err, ShouldBeNil)
				So(p.Value(), ShouldResembleV, []byte{0})

				nbf := BitField{}
				So(nbf.FromProperty(p), ShouldBeNil)
				So(nbf, ShouldResembleV, bf)
			})

			Convey("small sets correctly encode", func() {
				bf = Make(2)
				bf.Set(0)
				p, err := bf.ToProperty()
				So(err, ShouldBeNil)
				So(p.Value(), ShouldResembleV, []byte{2, 1})

				nbf := BitField{}
				So(nbf.FromProperty(p), ShouldBeNil)
				So(nbf, ShouldResembleV, bf)
			})
		})
	})
}
