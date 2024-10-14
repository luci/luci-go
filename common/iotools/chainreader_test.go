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
	"errors"
	"io"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type infiniteReader struct{}

func (r infiniteReader) Read(b []byte) (int, error) {
	for idx := 0; idx < len(b); idx++ {
		b[idx] = 0x55
	}
	return len(b), nil
}

type errorReader struct {
	error
}

func (e *errorReader) Read([]byte) (int, error) {
	return 0, e.error
}

func TestChainReader(t *testing.T) {
	ftt.Run(`An empty ChainReader`, t, func(t *ftt.Test) {
		cr := ChainReader{}

		t.Run(`Should successfully read into a zero-byte array.`, func(t *ftt.Test) {
			d := []byte{}
			count, err := cr.Read(d)
			assert.Loosely(t, count, should.BeZero)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Should fail with io.EOF during ReadByte.`, func(t *ftt.Test) {
			b, err := cr.ReadByte()
			assert.Loosely(t, b, should.BeZero)
			assert.Loosely(t, err, should.Equal(io.EOF))
		})

		t.Run(`Should have zero remaining bytes.`, func(t *ftt.Test) {
			assert.Loosely(t, cr.Remaining(), should.BeZero)
		})
	})

	ftt.Run(`A ChainReader with {{0x00, 0x01}, nil, nil, {0x02}, nil}`, t, func(t *ftt.Test) {
		cr := ChainReader{bytes.NewReader([]byte{0x00, 0x01}), nil, nil, bytes.NewReader([]byte{0x02}), nil}

		t.Run(`The ChainReader should have a Remaining count of 3.`, func(t *ftt.Test) {
			assert.Loosely(t, cr.Remaining(), should.Equal(3))
		})

		t.Run(`The ChainReader should read: []byte{0x00, 0x01, 0x02} for buffer size 3.`, func(t *ftt.Test) {
			data := make([]byte, 3)
			count, err := cr.Read(data)
			assert.Loosely(t, count, should.Equal(3))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, data, should.Resemble([]byte{0x00, 0x01, 0x02}))

			assert.Loosely(t, cr.Remaining(), should.BeZero)
		})

		t.Run(`The ChainReader should read: []byte{0x00, 0x01} for buffer size 2.`, func(t *ftt.Test) {
			data := make([]byte, 2)
			count, err := cr.Read(data)
			assert.Loosely(t, count, should.Equal(2))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, data, should.Resemble([]byte{0x00, 0x01}))

			assert.Loosely(t, cr.Remaining(), should.Equal(1))
		})

		t.Run(`The ChainReader should read bytes: 0x00, 0x01, 0x02, EOF.`, func(t *ftt.Test) {
			b, err := cr.ReadByte()
			assert.Loosely(t, b, should.Equal(0x00))
			assert.Loosely(t, err, should.BeNil)

			b, err = cr.ReadByte()
			assert.Loosely(t, b, should.Equal(0x01))
			assert.Loosely(t, err, should.BeNil)

			b, err = cr.ReadByte()
			assert.Loosely(t, b, should.Equal(0x02))
			assert.Loosely(t, err, should.BeNil)

			b, err = cr.ReadByte()
			assert.Loosely(t, b, should.Equal(0x00))
			assert.Loosely(t, err, should.Equal(io.EOF))

			assert.Loosely(t, cr.Remaining(), should.BeZero)
		})
	})

	ftt.Run(`A ChainReader with an infinite io.Reader`, t, func(t *ftt.Test) {
		cr := ChainReader{&infiniteReader{}}

		t.Run(`Should return an error on RemainingErr()`, func(t *ftt.Test) {
			_, err := cr.RemainingErr()
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`Should panic on Remaining()`, func(t *ftt.Test) {
			assert.Loosely(t, func() { cr.Remaining() }, should.Panic)
		})

		t.Run(`Should fill a 1024-byte buffer`, func(t *ftt.Test) {
			data := make([]byte, 1024)
			count, err := cr.Read(data)
			assert.Loosely(t, count, should.Equal(1024))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, data, should.Resemble(bytes.Repeat([]byte{0x55}, 1024)))
		})
	})

	ftt.Run(`A ChainReader with {0x00, 0x01} and an error-returning io.Reader`, t, func(t *ftt.Test) {
		e := errors.New("TEST ERROR")
		cr := ChainReader{bytes.NewReader([]byte{0x00, 0x01}), &errorReader{e}}

		t.Run(`Should fill a 3-byte buffer with the first two bytes and return an error.`, func(t *ftt.Test) {
			data := make([]byte, 3)
			count, err := cr.Read(data)
			assert.Loosely(t, count, should.Equal(2))
			assert.Loosely(t, err, should.Equal(e))
			assert.Loosely(t, data[:2], should.Resemble([]byte{0x00, 0x01}))
		})
	})
}
