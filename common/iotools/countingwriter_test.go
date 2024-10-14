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

// testWriter is an io.Writer and io.ByteWriter implementation that always
// writes the full amount and returns the configured error.
type testWriter struct {
	buf             bytes.Buffer
	writeByteCalled bool
	err             error
}

func (w *testWriter) Write(buf []byte) (int, error) {
	amt, _ := w.buf.Write(buf)
	return amt, w.err
}

func (w *testWriter) WriteByte(b byte) error {
	w.writeByteCalled = true

	if err := w.err; err != nil {
		return err
	}
	return w.buf.WriteByte(b)
}

type notAByteWriter struct {
	inner io.Writer
}

func (w *notAByteWriter) Write(buf []byte) (int, error) {
	return w.inner.Write(buf)
}

func TestCountingWriter(t *testing.T) {
	t.Parallel()

	ftt.Run(`A CountingWriter backed by a test writer`, t, func(t *ftt.Test) {
		tw := testWriter{}
		cw := CountingWriter{Writer: &tw}

		t.Run(`When writing 10 bytes of data, registers a count of 10.`, func(t *ftt.Test) {
			data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

			amount, err := cw.Write(data)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, amount, should.Equal(10))

			assert.Loosely(t, tw.buf.Bytes(), should.Resemble(data))
			assert.Loosely(t, cw.Count, should.Equal(10))
		})

		t.Run(`When using 32 sequential WriteByte, uses underlying WriteByte and registers a count of 32.`, func(t *ftt.Test) {
			written := bytes.Buffer{}

			for i := 0; i < 32; i++ {
				assert.Loosely(t, cw.WriteByte(byte(i)), should.BeNil)
				assert.Loosely(t, cw.Count, should.Equal(i+1))

				// Record for bulk comparison.
				written.WriteByte(byte(i))
			}

			assert.Loosely(t, tw.buf.Bytes(), should.Resemble(written.Bytes()))
			assert.Loosely(t, cw.Count, should.Equal(32))
			assert.Loosely(t, tw.writeByteCalled, should.BeTrue)
		})

		t.Run(`When an error is returned in Write, the error is propagated.`, func(t *ftt.Test) {
			tw.err = errors.New("test error")
			data := []byte{0, 1, 2, 3}

			amount, err := cw.Write(data)
			assert.Loosely(t, amount, should.Equal(len(data)))
			assert.Loosely(t, err, should.Equal(tw.err))
			assert.Loosely(t, tw.buf.Bytes(), should.Resemble(data))
			assert.Loosely(t, cw.Count, should.Equal(len(data)))
		})

		t.Run(`When an error is returned in WriteByte, the error is propagated.`, func(t *ftt.Test) {
			tw.err = errors.New("test error")

			err := cw.WriteByte(0x55)
			assert.Loosely(t, err, should.Equal(tw.err))
			assert.Loosely(t, tw.buf.Bytes(), should.HaveLength(0))
			assert.Loosely(t, cw.Count, should.BeZero)
			assert.Loosely(t, tw.writeByteCalled, should.BeTrue)
		})

		t.Run(`When WriteByte is disabled`, func(t *ftt.Test) {
			cw.Writer = &notAByteWriter{&tw}

			t.Run(`WriteByte calls the underlying Write and propagates test error.`, func(t *ftt.Test) {
				tw.err = errors.New("test error")

				err := cw.WriteByte(0x55)
				assert.Loosely(t, err, should.Equal(tw.err))
				assert.Loosely(t, tw.buf.Bytes(), should.Resemble([]byte{0x55}))
				assert.Loosely(t, cw.Count, should.Equal(1))
				assert.Loosely(t, tw.writeByteCalled, should.BeFalse)
			})
		})
	})
}
