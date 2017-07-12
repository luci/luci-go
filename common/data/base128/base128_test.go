// Copyright 2016 The LUCI Authors.
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

package base128

import (
	"bytes"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBase128(t *testing.T) {
	t.Parallel()

	Convey("base128", t, func() {
		Convey("lengths", func() {
			cases := []struct {
				dec, enc int
			}{
				{0, 0},
				{1, 2},
				{6, 7},
				{7, 8},
				{8, 10},
				{9, 11},
			}
			for _, c := range cases {
				Convey(fmt.Sprintf("enc:%d dec:%d", c.enc, c.dec), func() {
					So(DecodedLen(c.enc), ShouldEqual, c.dec)
					So(EncodedLen(c.dec), ShouldEqual, c.enc)
					So(EncodedLen(DecodedLen(c.enc)), ShouldEqual, c.enc)
					So(DecodedLen(EncodedLen(c.dec)), ShouldEqual, c.dec)
				})
			}
		})

		Convey("errors", func() {
			Convey("bad length", func() {
				_, err := Decode(nil, []byte{0xff})
				So(err, ShouldEqual, ErrLength)

				_, err = Decode(nil, bytes.Repeat([]byte{0xff}, 9))
				So(err, ShouldEqual, ErrLength)

				So(func() { Decode(nil, []byte{0xff, 0xff}) }, ShouldPanic)
			})

			Convey("bad byte", func() {
				_, err := Decode([]byte{0}, []byte{0x7f, 0xff})
				So(err, ShouldEqual, ErrBit)
			})
		})

		Convey("bytes", func() {
			cases := []struct {
				dec, enc []byte
			}{
				{[]byte{}, []byte{}},
				{[]byte{0xff}, []byte{0x7f, 0x40}},
				{
					bytes.Repeat([]byte{0xff}, 6),
					append(bytes.Repeat([]byte{0x7f}, 6), 0x7e),
				},
				{
					bytes.Repeat([]byte{0xff}, 7),
					bytes.Repeat([]byte{0x7f}, 8),
				},
				{
					bytes.Repeat([]byte{0xff}, 8),
					append(bytes.Repeat([]byte{0x7f}, 9), 0x40),
				},
			}
			for _, c := range cases {
				Convey(fmt.Sprintf("%q -> %q", c.dec, c.enc), func() {
					buf := make([]byte, len(c.enc))
					So(Encode(buf, c.dec), ShouldEqual, len(buf))
					So(buf, ShouldResemble, c.enc)

					buf = make([]byte, len(c.dec))
					n, err := Decode(buf, c.enc)
					So(err, ShouldBeNil)
					So(n, ShouldEqual, len(buf))
					So(buf, ShouldResemble, c.dec)
				})
			}
		})
	})
}
