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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	ftt.Run(`Given a CountingReader backed by a 32-byte not-ByteReader Reader.`, t, func(t *ftt.Test) {
		buf := bytes.NewBuffer(bytes.Repeat([]byte{0x55}, 32))
		tr := &notAByteReader{buf}
		cr := CountingReader{Reader: tr}

		t.Run(`When reading 10 bytes of data, registers a count of 10.`, func(t *ftt.Test) {
			amount, err := cr.Read(make([]byte, 10))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, amount, should.Equal(10))
			assert.Loosely(t, cr.Count, should.Equal(10))
		})

		t.Run(`When using 32 sequential ReadByte, registers a count of 32.`, func(t *ftt.Test) {
			for i := range 32 {
				b, err := cr.ReadByte()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b, should.Equal(0x55))
				assert.Loosely(t, cr.Count, should.Equal(i+1))
			}

			_, err := cr.ReadByte()
			assert.Loosely(t, err, should.Equal(io.EOF))
		})

		t.Run(`ReadByte should return EOF if no more data.`, func(t *ftt.Test) {
			buf.Reset()

			b, err := cr.ReadByte()
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, b, should.BeZero)
		})
	})

	ftt.Run(`Given a CountingReader backed by a testByteReader.`, t, func(t *ftt.Test) {
		tr := testByteReader{ByteReader: bytes.NewBuffer([]byte{0x55})}
		cr := CountingReader{Reader: &tr}

		t.Run(`ReadByte should directly call the backing reader's ReadByte.`, func(t *ftt.Test) {
			b, err := cr.ReadByte()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b, should.Equal(0x55))
			assert.Loosely(t, cr.Count, should.Equal(1))
			assert.Loosely(t, tr.called, should.BeTrue)
		})
	})
}
