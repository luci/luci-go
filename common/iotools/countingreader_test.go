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
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type notAByteReader struct {
	io.Reader
}

func (r *notAByteReader) Read(buf []byte) (int, error) {
	return r.Reader.Read(buf)
}

// Testing byte reader.
type testByteReader struct {
	io.ByteReader
	called bool
}

func (r *testByteReader) Read([]byte) (int, error) {
	panic("Not implemented.")
}

func (r *testByteReader) ReadByte() (byte, error) {
	r.called = true
	return r.ByteReader.ReadByte()
}

func TestCountingReader(t *testing.T) {
	Convey(`Given a CountingReader backed by a 32-byte not-ByteReader Reader.`, t, func() {
		buf := bytes.NewBuffer(bytes.Repeat([]byte{0x55}, 32))
		tr := &notAByteReader{buf}
		cr := CountingReader{Reader: tr}

		Convey(`When reading 10 bytes of data, registers a count of 10.`, func() {
			amount, err := cr.Read(make([]byte, 10))
			So(err, ShouldBeNil)
			So(amount, ShouldEqual, 10)
			So(cr.Count, ShouldEqual, 10)
		})

		Convey(`When using 32 sequential ReadByte, registers a count of 32.`, func() {
			for i := 0; i < 32; i++ {
				b, err := cr.ReadByte()
				So(err, ShouldBeNil)
				So(b, ShouldEqual, 0x55)
				So(cr.Count, ShouldEqual, i+1)
			}

			_, err := cr.ReadByte()
			So(err, ShouldEqual, io.EOF)
		})

		Convey(`ReadByte should return EOF if no more data.`, func() {
			buf.Reset()

			b, err := cr.ReadByte()
			So(err, ShouldEqual, io.EOF)
			So(b, ShouldEqual, 0)
		})
	})

	Convey(`Given a CountingReader backed by a testByteReader.`, t, func() {
		tr := testByteReader{ByteReader: bytes.NewBuffer([]byte{0x55})}
		cr := CountingReader{Reader: &tr}

		Convey(`ReadByte should directly call the backing reader's ReadByte.`, func() {
			b, err := cr.ReadByte()
			So(err, ShouldBeNil)
			So(b, ShouldEqual, 0x55)
			So(cr.Count, ShouldEqual, 1)
			So(tr.called, ShouldBeTrue)
		})
	})
}
