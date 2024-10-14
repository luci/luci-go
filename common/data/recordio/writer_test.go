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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testWriter struct {
	buf  bytes.Buffer
	errQ []error
}

func (w *testWriter) Write(data []byte) (int, error) {
	if len(w.errQ) > 0 {
		err := w.errQ[0]
		w.errQ = w.errQ[1:]

		if err != nil {
			return 0, err
		}
	}
	return w.buf.Write(data)
}

func TestWriteFrame(t *testing.T) {
	t.Parallel()

	ftt.Run(`Using a buffered test writer`, t, func(t *ftt.Test) {
		tw := &testWriter{}

		t.Run(`WriteFrame will successfully encode a zero-byte frame.`, func(t *ftt.Test) {
			count, err := WriteFrame(tw, []byte{})
			assert.Loosely(t, count, should.Equal(1))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tw.buf.Bytes(), should.Resemble([]byte{0x00}))
		})

		t.Run(`WriteFrame will successfully encode a 129-byte frame.`, func(t *ftt.Test) {
			data := bytes.Repeat([]byte{0x5A}, 129)

			count, err := WriteFrame(tw, data)
			assert.Loosely(t, count, should.Equal(131))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tw.buf.Bytes(), should.Resemble(append([]byte{0x81, 0x01}, data...)))
		})

		t.Run(`Will return an error when failing to write the size header.`, func(t *ftt.Test) {
			failErr := errors.New("test: test-induced size error")
			tw.errQ = []error{
				failErr,
			}

			count, err := WriteFrame(tw, []byte{0xd0, 0x65})
			assert.Loosely(t, count, should.BeZero)
			assert.Loosely(t, err, should.Equal(failErr))
		})

		t.Run(`Will return an error when failing to write the frame data.`, func(t *ftt.Test) {
			failErr := errors.New("test: test-induced size error")
			tw.errQ = []error{
				nil,
				failErr,
			}

			count, err := WriteFrame(tw, []byte{0xd0, 0x65})
			assert.Loosely(t, count, should.Equal(1))
			assert.Loosely(t, err, should.Equal(failErr))
		})
	})
}

func TestWriter(t *testing.T) {
	t.Parallel()

	ftt.Run(`A Writer, configured to write to a buffer`, t, func(t *ftt.Test) {
		tw := &testWriter{}
		w := NewWriter(tw)

		t.Run(`Can write consecutive frames in 3-byte chunks.`, func(t *ftt.Test) {
			expected := []byte{}
			var sizeBuf [binary.MaxVarintLen64]byte
			for _, size := range []int{
				1,
				0,
				1025,
				129,
				11,
			} {
				b := bytes.Repeat([]byte{0x55}, size)
				expected = append(expected, sizeBuf[:binary.PutUvarint(sizeBuf[:], uint64(size))]...)
				expected = append(expected, b...)

				for len(b) > 0 {
					count := 3
					if count > len(b) {
						count = len(b)
					}

					c, err := w.Write(b[:count])
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, c, should.Equal(count))

					b = b[count:]
				}
				assert.Loosely(t, w.Flush(), should.BeNil)
			}

			assert.Loosely(t, tw.buf.Bytes(), should.Resemble(expected))
		})

		t.Run(`Will write empty frames if Flush()ed.`, func(t *ftt.Test) {
			assert.Loosely(t, w.Flush(), should.BeNil)
			assert.Loosely(t, w.Flush(), should.BeNil)
			assert.Loosely(t, w.Flush(), should.BeNil)
			assert.Loosely(t, tw.buf.Bytes(), should.Resemble([]byte{0x00, 0x00, 0x00}))
		})

		t.Run(`Will fail to Flush() if the Write fails.`, func(t *ftt.Test) {
			failErr := errors.New("test: test-induced size error")
			tw.errQ = []error{
				failErr,
			}
			assert.Loosely(t, w.Flush(), should.Equal(failErr))
		})
	})
}
