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

package recordio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// plainReader implements the io.Reader interface on top of a bytes.Buffer;
// however, it does not also implement io.ByteReader.
type plainReader struct {
	buf bytes.Buffer
	err error
}

func (r *plainReader) loadFrames(chunks ...[]byte) {
	for _, chunk := range chunks {
		_, err := WriteFrame(&r.buf, chunk)
		if err != nil {
			panic(err)
		}
	}
}

func (r *plainReader) Read(data []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.buf.Read(data)
}

type testByteReader struct {
	plainReader

	readByteErr error
	readBytes   int
}

func (r *testByteReader) ReadByte() (b byte, err error) {
	if r.readByteErr != nil {
		return 0, r.readByteErr
	}

	b, err = r.buf.ReadByte()
	if err == nil {
		r.readBytes++
	}
	return
}

func btos(b ...[]byte) []string {
	s := make([]string, len(b))
	for i, v := range b {
		s[i] = string(v)
	}
	return s
}

// TestReader tests the default Reader implementation, "reader".
func TestReader(t *testing.T) {
	t.Parallel()

	ftt.Run(`A frame reader with max size 1MB using a plain io.Reader`, t, func(t *ftt.Test) {
		maxSize := int64(1024 * 1024)
		tr := plainReader{}
		r := NewReader(&tr, maxSize)

		t.Run(`Will return io.EOF with an empty reader.`, func(t *ftt.Test) {
			_, err := r.ReadFrameAll()
			assert.Loosely(t, err, should.Equal(io.EOF))
		})

		t.Run(`Can successfully read a frame.`, func(t *ftt.Test) {
			data := []byte{0x13, 0x37, 0xd0, 0x65}
			tr.loadFrames(data)

			f, err := r.ReadFrameAll()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f, should.Match(data))
		})

		t.Run(`Can successfully read two frames.`, func(t *ftt.Test) {
			data := [][]byte{
				{0x13, 0x37, 0xd0, 0x65},
				{0xd0, 0x06, 0xea, 0x15, 0xf0, 0x0d},
			}
			tr.loadFrames(data...)

			c, fr, err := r.ReadFrame()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, c, should.Equal(4))

			d, err := io.ReadAll(fr)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, d, should.Match(data[0]))

			c, fr, err = r.ReadFrame()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, c, should.Equal(6))

			d, err = io.ReadAll(fr)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, d, should.Match(data[1]))
		})

		t.Run(`When reading a frame, will return EOF if the frame is exceeded.`, func(t *ftt.Test) {
			data := []byte{0x13, 0x37, 0xd0, 0x65}
			tr.loadFrames(data)

			count, fr, err := r.ReadFrame()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, count, should.Equal(4))

			buf := make([]byte, 5)
			c, err := fr.Read(make([]byte, 5))
			assert.Loosely(t, c, should.Equal(4))
			assert.Loosely(t, err, should.BeNil)

			buf = buf[count:]
			_, err = fr.Read(buf)
			assert.Loosely(t, err, should.Equal(io.EOF))
		})

		t.Run(`Will fail if the underlying frame exceeds the maximum size.`, func(t *ftt.Test) {
			var sizeBuf [binary.MaxVarintLen64]byte
			tr.buf.Write(sizeBuf[:binary.PutUvarint(sizeBuf[:], uint64(maxSize+1))])

			_, err := r.ReadFrameAll()
			assert.Loosely(t, err, should.Equal(ErrFrameTooLarge))
		})

		t.Run(`Will fail if the frame contains an invalid size header.`, func(t *ftt.Test) {
			tr.buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
			_, err := r.ReadFrameAll()
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`Can read conscutive frames, then io.EOF.`, func(t *ftt.Test) {
			data := [][]byte{}
			for _, size := range []int{
				0,
				14,
				1024 * 1024,
				0,
				511,
			} {
				data = append(data, bytes.Repeat([]byte{0x5A}, size))
				tr.loadFrames(data[len(data)-1])
			}

			for _, expected := range data {
				f, err := r.ReadFrameAll()
				assert.Loosely(t, err, should.BeNil)

				if len(expected) == 0 {
					expected = nil
				}
				assert.Loosely(t, f, should.Match(expected))
			}

			_, err := r.ReadFrameAll()
			assert.Loosely(t, err, should.Equal(io.EOF))
		})
	})

	ftt.Run(`A frame reader with max size 1MB using an io.Reader+io.ByteReader`, t, func(t *ftt.Test) {
		tr := testByteReader{}
		r := NewReader(&tr, 1024*1024)

		t.Run(`Will return io.EOF with an empty reader.`, func(t *ftt.Test) {
			_, err := r.ReadFrameAll()
			assert.Loosely(t, err, should.Equal(io.EOF))
		})

		t.Run(`Will use io.ByteReader to read the frame header.`, func(t *ftt.Test) {
			data := []byte{0x13, 0x37, 0xd0, 0x65}
			tr.loadFrames(data)

			f, err := r.ReadFrameAll()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f, should.Match(data))
			assert.Loosely(t, tr.readBytes, should.Equal(1))
		})

		t.Run(`Will fail if the underlying io.Reader returns an error.`, func(t *ftt.Test) {
			tr.loadFrames([]byte{})
			tr.err = errors.New("test: test-induced error")
			tr.readByteErr = tr.err
			_, err := r.ReadFrameAll()
			assert.Loosely(t, err, should.Equal(tr.err))
		})

		t.Run(`Will fail if an error is returned while reading frame's data.`, func(t *ftt.Test) {
			data := []byte{0x13, 0x37, 0xd0, 0x65}
			tr.loadFrames(data)

			// Have "ReadByte()" calls ignore the configured error. This will cause
			// the frame size to be read without incident, but the frame data to still
			// return an error.
			tr.err = errors.New("test: test-induced error")
			data, err := r.ReadFrameAll()
			assert.Loosely(t, err, should.Equal(tr.err))
		})
	})
}

func TestSplit(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing Split`, t, func(t *ftt.Test) {
		for _, v := range [][]string{
			{},
			{""},
			{"", "foo", ""},
			{"foo", "bar", "baz"},
		} {
			t.Run(fmt.Sprintf(`Can Split stream: %#v`, v), func(t *ftt.Test) {
				// Write frames to "buf".
				var buf bytes.Buffer
				for _, s := range v {
					_, err := WriteFrame(&buf, []byte(s))
					if err != nil {
						panic(err)
					}
				}

				// Confirm that Split works.
				sp, err := Split(buf.Bytes())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, btos(sp...), should.Match(v))
			})
		}

		t.Run(`Will refuse to split a frame that is too large.`, func(t *ftt.Test) {
			_, err := Split([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00})
			assert.Loosely(t, err, should.Equal(ErrFrameTooLarge))
		})

		t.Run(`Will fail to split if there aren't enough bytes.`, func(t *ftt.Test) {
			sp, err := Split([]byte{0x01, 0xAA, 0x02}) // 1-byte {0xAA}, 2-bytes ... EOF!
			assert.Loosely(t, sp, should.Match([][]byte{{0xAA}}))
			assert.Loosely(t, err, should.Equal(ErrFrameTooLarge))
		})
	})
}
