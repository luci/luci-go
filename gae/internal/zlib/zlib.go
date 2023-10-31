// Copyright 2023 The LUCI Authors.
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

// Package zlib holds zlib decoders used by the GAE library.
package zlib

import (
	"bytes"
	"io"

	"github.com/klauspost/compress/zlib"
)

// DecodeAll allows stateless decoding of a blob of bytes.
//
// Output will be appended to dst and the resulting slice will be returned, so
// if the destination size is known you can pre-allocate the destination slice
// to avoid allocations. DecodeAll can be used concurrently.
func DecodeAll(input, dst []byte) ([]byte, error) {
	// TODO(vadimsh): zlib reader can theoretically be reused similar to how we
	// reuse zstd decoders, but considering zlib is used only in legacy code
	// paths, it is probably not worth doing.
	r, err := zlib.NewReader(bytes.NewBuffer(input))
	if err != nil {
		return dst, err
	}
	w := bytes.NewBuffer(dst)
	if _, err := io.Copy(w, r); err != nil {
		_ = r.Close()
		return w.Bytes(), err
	}
	return w.Bytes(), r.Close()
}

// HasZlibHeader checks for presence of zlib headers.
func HasZlibHeader(blob []byte) bool {
	// Based on https://github.com/googleapis/python-ndb/blob/b0f431048b7b/google/cloud/ndb/model.py#L338
	return len(blob) >= 2 && blob[0] == 0x78 && (blob[1] == 0x9c || blob[1] == 0x5e)
}
