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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestByteSliceReader(t *testing.T) {
	ftt.Run(`A ByteSliceReader for {0x60, 0x0d, 0xd0, 0x65}`, t, func(t *ftt.Test) {
		data := []byte{0x60, 0x0d, 0xd0, 0x65}
		bsd := ByteSliceReader(data)

		t.Run(`Can read byte-by-byte.`, func(t *ftt.Test) {
			b, err := bsd.ReadByte()
			assert.Loosely(t, b, should.Equal(0x60))
			assert.Loosely(t, err, should.BeNil)

			b, err = bsd.ReadByte()
			assert.Loosely(t, b, should.Equal(0x0d))
			assert.Loosely(t, err, should.BeNil)

			b, err = bsd.ReadByte()
			assert.Loosely(t, b, should.Equal(0xd0))
			assert.Loosely(t, err, should.BeNil)

			b, err = bsd.ReadByte()
			assert.Loosely(t, b, should.Equal(0x65))
			assert.Loosely(t, err, should.BeNil)

			b, err = bsd.ReadByte()
			assert.Loosely(t, err, should.Equal(io.EOF))
		})

		t.Run(`Can read the full array.`, func(t *ftt.Test) {
			buf := make([]byte, 4)
			count, err := bsd.Read(buf)
			assert.Loosely(t, count, should.Equal(4))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf, should.Match(data))
		})

		t.Run(`When read into an oversized buf, returns io.EOF and setting the slice to nil.`, func(t *ftt.Test) {
			buf := make([]byte, 16)
			count, err := bsd.Read(buf)
			assert.Loosely(t, count, should.Equal(4))
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, buf[:count], should.Match(data))
			assert.Loosely(t, []byte(bsd), should.BeNil)
		})

		t.Run(`When read in two 3-byte parts, the latter returns io.EOF and sets the slice to nil.`, func(t *ftt.Test) {
			buf := make([]byte, 3)

			count, err := bsd.Read(buf)
			assert.Loosely(t, count, should.Equal(3))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf, should.Match(data[:3]))
			assert.Loosely(t, []byte(bsd), should.Match(data[3:]))

			count, err = bsd.Read(buf)
			assert.Loosely(t, count, should.Equal(1))
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, buf[:count], should.Match(data[3:]))
			assert.Loosely(t, []byte(bsd), should.BeNil)
		})
	})

	ftt.Run(`A ByteSliceReader for nil`, t, func(t *ftt.Test) {
		bsd := ByteSliceReader(nil)

		t.Run(`A byte read will return io.EOF.`, func(t *ftt.Test) {
			_, err := bsd.ReadByte()
			assert.Loosely(t, err, should.Equal(io.EOF))
		})
	})

	ftt.Run(`A ByteSliceReader for an empty byte array`, t, func(t *ftt.Test) {
		bsd := ByteSliceReader([]byte{})

		t.Run(`A byte read will return io.EOF and set the array to nil.`, func(t *ftt.Test) {
			_, err := bsd.ReadByte()
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, []byte(bsd), should.BeNil)
		})
	})
}
