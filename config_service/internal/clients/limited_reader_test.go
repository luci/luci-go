// Copyright 2026 The LUCI Authors.
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

package clients

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/klauspost/compress/gzip"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type fakeObjIdent struct {
	bucket string
	object string
}

func (f fakeObjIdent) BucketName() string { return f.bucket }
func (f fakeObjIdent) ObjectName() string { return f.object }

type errReadCloser struct {
	r        io.Reader
	readErr  error
	closeErr error
	closed   bool
}

func (e *errReadCloser) Read(p []byte) (int, error) {
	if e.r != nil {
		n, err := e.r.Read(p)
		if err != io.EOF || e.readErr == nil {
			return n, err
		}
		return n, e.readErr
	}
	return 0, e.readErr
}

func (e *errReadCloser) Close() error {
	e.closed = true
	return e.closeErr
}

func gzipBytes(t testing.TB, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, err := gw.Write(raw)
	assert.NoErr(t, err)
	assert.NoErr(t, gw.Close())
	return buf.Bytes()
}

func TestLimitedReader(t *testing.T) {
	t.Parallel()

	ident := fakeObjIdent{bucket: "bkt", object: "obj"}

	t.Run("reads within and at exact limit", func(t *testing.T) {
		t.Parallel()
		payload := []byte("hello world")
		gz := gzipBytes(t, payload)
		gr, err := gzip.NewReader(bytes.NewReader(gz))
		assert.NoErr(t, err)

		var underlyingClosed bool
		lr := &limitedReader{
			object:          ident,
			r:               gr,
			underlyingClose: func() error { underlyingClosed = true; return nil },
			limit:           len(payload),
		}

		got, err := io.ReadAll(lr)
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(payload))
		assert.NoErr(t, lr.Close())
		assert.That(t, underlyingClosed, should.BeTrue)
	})

	t.Run("returns ErrTooLarge when exceeding limit by 1 byte", func(t *testing.T) {
		t.Parallel()
		payload := []byte("hello world!")
		gz := gzipBytes(t, payload)
		gr, err := gzip.NewReader(bytes.NewReader(gz))
		assert.NoErr(t, err)

		lr := &limitedReader{
			object:          ident,
			r:               gr,
			underlyingClose: func() error { return nil },
			limit:           len(payload) - 1,
		}

		_, err = io.ReadAll(lr)
		assert.That(t, errors.Is(err, ErrTooLarge), should.BeTrue)
		assert.ErrIsLike(t, err, "gs://bkt/obj: gcs object too large: 12 > 11")

		// Subsequent reads keep returning ErrTooLarge.
		_, err = lr.Read(make([]byte, 8))
		assert.That(t, errors.Is(err, ErrTooLarge), should.BeTrue)
	})

	t.Run("wraps truncated gzip stream with ErrInvalidGzip", func(t *testing.T) {
		t.Parallel()
		gz := gzipBytes(t, []byte("some compressed content"))
		// Truncate trailer/body after valid 10-byte gzip header.
		gr, err := gzip.NewReader(bytes.NewReader(gz[:len(gz)-4]))
		assert.NoErr(t, err)

		lr := &limitedReader{
			object:          ident,
			r:               gr,
			underlyingClose: func() error { return nil },
			limit:           1024,
		}

		_, err = io.ReadAll(lr)
		assert.That(t, errors.Is(err, ErrInvalidGzip), should.BeTrue)
	})

	t.Run("wraps corrupt gzip checksum with ErrInvalidGzip", func(t *testing.T) {
		t.Parallel()
		gz := gzipBytes(t, []byte("some compressed content"))
		// Flip a byte in the CRC32 trailer (last 8 bytes are CRC32 + ISIZE).
		gz[len(gz)-8] ^= 0xff
		gr, err := gzip.NewReader(bytes.NewReader(gz))
		assert.NoErr(t, err)

		lr := &limitedReader{
			object:          ident,
			r:               gr,
			underlyingClose: func() error { return nil },
			limit:           1024,
		}

		_, err = io.ReadAll(lr)
		assert.That(t, errors.Is(err, ErrInvalidGzip), should.BeTrue)
		assert.That(t, errors.Is(err, gzip.ErrChecksum), should.BeTrue)
	})

	t.Run("wraps corrupt flate stream with ErrInvalidGzip", func(t *testing.T) {
		t.Parallel()
		gz := gzipBytes(t, bytes.Repeat([]byte("abcdef0123456789"), 64))
		// Corrupt deflate body right after 10-byte gzip header.
		gz[10] = 0xff
		gz[11] = 0xff
		gz[12] = 0xff
		gr, err := gzip.NewReader(bytes.NewReader(gz))
		assert.NoErr(t, err)

		lr := &limitedReader{
			object:          ident,
			r:               gr,
			underlyingClose: func() error { return nil },
			limit:           4096,
		}

		_, err = io.ReadAll(lr)
		assert.That(t, errors.Is(err, ErrInvalidGzip), should.BeTrue)
	})

	t.Run("does not wrap transport error as ErrInvalidGzip", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("connection reset by peer")
		rc := &errReadCloser{readErr: boom, closeErr: errors.New("close1")}
		close2 := errors.New("close2")

		lr := &limitedReader{
			object:          ident,
			r:               rc,
			underlyingClose: func() error { return close2 },
			limit:           1024,
		}

		_, err := io.ReadAll(lr)
		assert.That(t, errors.Is(err, boom), should.BeTrue)
		assert.That(t, errors.Is(err, ErrInvalidGzip), should.BeFalse)

		closeErr := lr.Close()
		assert.That(t, rc.closed, should.BeTrue)
		assert.That(t, errors.Is(closeErr, rc.closeErr), should.BeTrue)
		assert.That(t, errors.Is(closeErr, close2), should.BeTrue)
	})

	t.Run("newLimitedReader wraps header gzip errors and empty input as ErrInvalidGzip", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			raw  []byte
		}{
			{name: "empty 0-byte object", raw: nil},
			{name: "truncated header", raw: []byte{0x1f, 0x8b}},
			{name: "invalid magic", raw: []byte("not gzip header bytes")},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				rc := &errReadCloser{r: bytes.NewReader(tc.raw)}
				_, err := newLimitedReader(ident, rc, storage.ReaderObjectAttrs{}, 1024)
				assert.That(t, errors.Is(err, ErrInvalidGzip), should.BeTrue)
				assert.That(t, rc.closed, should.BeTrue)
			})
		}
	})

	t.Run("newLimitedReader does not wrap header transport or context errors as ErrInvalidGzip", func(t *testing.T) {
		t.Parallel()
		for _, wantErr := range []error{context.Canceled, errors.New("connection reset during header")} {
			rc := &errReadCloser{
				r:       bytes.NewReader([]byte{0x1f, 0x8b}), // fails mid-header read
				readErr: wantErr,
			}
			_, err := newLimitedReader(ident, rc, storage.ReaderObjectAttrs{}, 1024)
			assert.That(t, errors.Is(err, wantErr), should.BeTrue)
			assert.That(t, errors.Is(err, ErrInvalidGzip), should.BeFalse)
			assert.That(t, rc.closed, should.BeTrue)
		}
	})
}
