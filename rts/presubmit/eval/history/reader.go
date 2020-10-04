// Copyright 2020 The LUCI Authors.
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

package history

import (
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/recordio"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Reader deserializes historical records from an io.Reader.
type Reader struct {
	buf  []byte
	src  io.Reader
	rio  recordio.Reader
	zstd *zstd.Decoder
}

// NewReader creates a Reader.
func NewReader(r io.Reader) *Reader {
	ret := &Reader{src: r}
	ret.zstd, _ = zstd.NewReader(r)             // cannot return error - no options
	ret.rio = recordio.NewReader(ret.zstd, 1e7) // max 10MB proto.
	return ret
}

// OpenFile creates a Reader that reads data from a file.
// When done, call Close() on the returned Reader.
func OpenFile(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return NewReader(f), nil
}

// Read reads the next historical record.
// Returns io.EOF if there is no record.
func (r *Reader) Read() (*evalpb.Record, error) {
	// Read the next frame into the buffer.
	size, frame, err := r.rio.ReadFrame()
	switch {
	case err != nil:
		return nil, err
	case cap(r.buf) < int(size):
		r.buf = make([]byte, size)
	default:
		r.buf = r.buf[:size]
	}
	if _, err := io.ReadFull(frame, r.buf); err != nil {
		return nil, err
	}

	// Unmrashal and return the record.
	rec := &evalpb.Record{}
	if err := proto.Unmarshal(r.buf, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// Close releases all resources and closes the underlying io.Reader.
func (r *Reader) Close() error {
	r.zstd.Close()

	if closer, ok := r.src.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
