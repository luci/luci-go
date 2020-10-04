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
	"go.chromium.org/luci/common/errors"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Writer serializes historic facts to an io.Writer.
type Writer struct {
	buf    []byte
	closer io.Closer
	rio    recordio.Writer
	zstd   *zstd.Encoder
}

// NewWriter creates a HistoryWriter.
func NewWriter(w io.Writer) *Writer {
	var ret Writer
	ret.closer, _ = w.(io.Closer)

	var err error
	if ret.zstd, err = zstd.NewWriter(w); err != nil {
		panic(err) // we don't pass any options
	}

	ret.rio = recordio.NewWriter(ret.zstd)
	return &ret
}

// CreateFile returns HistoryWriter that persists data to a new file.
// When done, call Close() on the returned Writer.
func CreateFile(path string) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return NewWriter(f), nil
}

// Write writes a historical record.
func (w *Writer) Write(rec *evalpb.Record) error {
	// Marshal the record reusing the buffer.
	marshalled, err := (&proto.MarshalOptions{}).MarshalAppend(w.buf, rec)
	if err != nil {
		return errors.Annotate(err, "failed to marshal proto").Err()
	}
	// Use the larger buffer if the previous was too small.
	w.buf = marshalled[:0]

	if _, err := w.rio.Write(marshalled); err != nil {
		return err
	}
	return w.rio.Flush()
}

// Close flushes everything and closes the underlying io.Writer.
func (w *Writer) Close() error {
	if err := w.zstd.Close(); err != nil {
		return err
	}

	if w.closer != nil {
		return w.closer.Close()
	}
	return nil
}
