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
	"fmt"

	"cloud.google.com/go/storage"

	"go.chromium.org/luci/config_service/internal/common"
)

// limitedReader limits how much decompressed data we're willing to read from
// a gzip compressed GCS object.
type limitedReader struct {
	object interface {
		BucketName() string
		ObjectName() string
	}
	r     *storage.Reader
	n     int
	limit int
}

func (l *limitedReader) Read(buf []byte) (n int, err error) {
	if left := l.limit - l.n; left < len(buf) {
		buf = buf[:left+1] // Allow going over by 1 byte.
	}

	n, err = l.r.Read(buf)
	l.n += n
	if l.n > l.limit {
		err = fmt.Errorf("gs://%s/%s: %w: %d > %d",
			l.object.BucketName(), l.object.ObjectName(), ErrTooLarge, l.n, l.limit)
	}
	return
}

func (l *limitedReader) Close() error {
	return l.r.Close()
}

func (l *limitedReader) Attrs() storage.ReaderObjectAttrs {
	return l.r.Attrs
}

func newReader(ctx context.Context, object *storage.ObjectHandle) (*limitedReader, error) {
	r, err := object.NewReader(ctx)
	if err != nil {
		return nil, err
	}
	return &limitedReader{object, r, 0, common.ConfigMaxSize}, nil
}
