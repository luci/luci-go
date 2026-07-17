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

// Package decompress provides utilities for decompressing data.
package decompress

import (
	"bytes"
	"io"

	"github.com/klauspost/compress/gzip"

	"go.chromium.org/luci/common/errors"
)

// ConfigMaxSize is the maximum size of decompressed config supported.
const ConfigMaxSize = 200 * 1024 * 1024 // 200 MiB

// Gzip decompresses gzip-compressed data.
func Gzip(data []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Fmt("failed to create gzip reader: %w", err)
	}
	ret, err := io.ReadAll(io.LimitReader(gr, ConfigMaxSize+1))
	if err != nil {
		_ = gr.Close()
		return nil, errors.Fmt("failed to decompress the data: %w", err)
	}
	if err := gr.Close(); err != nil {
		return nil, errors.Fmt("errors closing gzip reader: %w", err)
	}
	if int64(len(ret)) > ConfigMaxSize {
		return nil, errors.Fmt("decompressed data exceeds max size %d bytes", ConfigMaxSize)
	}
	return ret, nil
}
