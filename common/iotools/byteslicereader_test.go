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

package iotools

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestByteSliceReader(t *testing.T) {
	Convey(`A ByteSliceReader for {0x60, 0x0d, 0xd0, 0x65}`, t, func() {
		data := []byte{0x60, 0x0d, 0xd0, 0x65}
		bsd := ByteSliceReader(data)

		Convey(`Can read byte-by-byte.`, func() {
			b, err := bsd.ReadByte()
			So(b, ShouldEqual, 0x60)
			So(err, ShouldBeNil)

			b, err = bsd.ReadByte()
			So(b, ShouldEqual, 0x0d)
			So(err, ShouldBeNil)

			b, err = bsd.ReadByte()
			So(b, ShouldEqual, 0xd0)
			So(err, ShouldBeNil)

			b, err = bsd.ReadByte()
			So(b, ShouldEqual, 0x65)
			So(err, ShouldBeNil)

			b, err = bsd.ReadByte()
			So(err, ShouldEqual, io.EOF)
		})

		Convey(`Can read the full array.`, func() {
			buf := make([]byte, 4)
			count, err := bsd.Read(buf)
			So(count, ShouldEqual, 4)
			So(err, ShouldBeNil)
			So(buf, ShouldResemble, data)
		})

		Convey(`When read into an oversized buf, returns io.EOF and setting the slice to nil.`, func() {
			buf := make([]byte, 16)
			count, err := bsd.Read(buf)
			So(count, ShouldEqual, 4)
			So(err, ShouldEqual, io.EOF)
			So(buf[:count], ShouldResemble, data)
			So([]byte(bsd), ShouldBeNil)
		})

		Convey(`When read in two 3-byte parts, the latter returns io.EOF and sets the slice to nil.`, func() {
			buf := make([]byte, 3)

			count, err := bsd.Read(buf)
			So(count, ShouldEqual, 3)
			So(err, ShouldBeNil)
			So(buf, ShouldResemble, data[:3])
			So([]byte(bsd), ShouldResemble, data[3:])

			count, err = bsd.Read(buf)
			So(count, ShouldEqual, 1)
			So(err, ShouldEqual, io.EOF)
			So(buf[:count], ShouldResemble, data[3:])
			So([]byte(bsd), ShouldBeNil)
		})
	})

	Convey(`A ByteSliceReader for nil`, t, func() {
		bsd := ByteSliceReader(nil)

		Convey(`A byte read will return io.EOF.`, func() {
			_, err := bsd.ReadByte()
			So(err, ShouldEqual, io.EOF)
		})
	})

	Convey(`A ByteSliceReader for an empty byte array`, t, func() {
		bsd := ByteSliceReader([]byte{})

		Convey(`A byte read will return io.EOF and set the array to nil.`, func() {
			_, err := bsd.ReadByte()
			So(err, ShouldEqual, io.EOF)
			So([]byte(bsd), ShouldBeNil)
		})
	})
}
