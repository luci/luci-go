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
	"context"
	"errors"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/gzip"

	"go.chromium.org/luci/config_service/internal/common"
)

func isGzipErr(err error) bool {
	// NOTE: gzip uses flate internally which can return flate.CorruptInputError.
	_, isCorruptFlate := errors.AsType[flate.CorruptInputError](err)
	return isCorruptFlate ||
		errors.Is(err, gzip.ErrHeader) ||
		errors.Is(err, gzip.ErrChecksum) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

type gsObjectIdent interface {
	BucketName() string
	ObjectName() string
}

// limitedReader limits how much decompressed data we're willing to read from
// a gzip compressed GCS object.
type limitedReader struct {
	object          gsObjectIdent
	r               io.ReadCloser
	underlyingClose func() error
	attrs           storage.ReaderObjectAttrs
	n               int
	limit           int
}

func (l *limitedReader) Read(buf []byte) (n int, err error) {
	if l.n > l.limit {
		return 0, fmt.Errorf("gs://%s/%s: %w: %d > %d",
			l.object.BucketName(), l.object.ObjectName(), ErrTooLarge, l.n, l.limit)
	}
	if left := l.limit - l.n; left < len(buf) {
		buf = buf[:left+1] // Allow going over by 1 byte.
	}

	n, err = l.r.Read(buf)
	l.n += n
	switch {
	case l.n > l.limit:
		err = fmt.Errorf("gs://%s/%s: %w: %d > %d",
			l.object.BucketName(), l.object.ObjectName(), ErrTooLarge, l.n, l.limit)
	case isGzipErr(err):
		err = fmt.Errorf("gs://%s/%s: %w: %w",
			l.object.BucketName(), l.object.ObjectName(), ErrInvalidGzip, err)
	}
	return
}

func (l *limitedReader) Close() error {
	err1 := l.r.Close()
	err2 := l.underlyingClose()
	return errors.Join(err1, err2)
}

func (l *limitedReader) Attrs() storage.ReaderObjectAttrs {
	return l.attrs
}

func newLimitedReader(object gsObjectIdent, r io.ReadCloser, attrs storage.ReaderObjectAttrs, limit int) (*limitedReader, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		_ = r.Close()
		// gzip.NewReader returns io.EOF when the underlying object is 0 bytes (or
		// ends mid-header string), which is invalid gzip rather than a transport
		// error. Non-gzip errors (e.g. context cancellation or network failures)
		// are returned unwrapped so callers do not treat the object as corrupt.
		if isGzipErr(err) || errors.Is(err, io.EOF) {
			err = fmt.Errorf("gs://%s/%s: %w: %w",
				object.BucketName(), object.ObjectName(), ErrInvalidGzip, err)
		}
		return nil, err
	}
	return &limitedReader{object, gr, r.Close, attrs, 0, limit}, nil
}

func newReader(ctx context.Context, object *storage.ObjectHandle) (*limitedReader, error) {
	r, err := object.ReadCompressed(true).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	return newLimitedReader(object, r, r.Attrs, common.ConfigMaxSize)
}
