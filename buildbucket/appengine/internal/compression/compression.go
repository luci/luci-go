// Copyright 2022 The LUCI Authors.
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

// Package compression provides different compression and decompression
// functions for the usages in the buildbucket.
package compression

import (
	"bytes"
	"compress/zlib"
	"io"

	"github.com/klauspost/compress/zstd"

	"go.chromium.org/luci/common/errors"
)

// Globally shared zstd encoder and decoder. We use only their EncodeAll and
// DecodeAll methods which are allowed to be used concurrently. Internally, both
// the encode and the decoder have worker pools (limited by GOMAXPROCS) and each
// concurrent EncodeAll/DecodeAll call temporary consumes one worker (so overall
// we do not run more jobs that we have cores for).
var (
	zstdEncoder *zstd.Encoder
	zstdDecoder *zstd.Decoder
)

func init() {
	var err error
	if zstdEncoder, err = zstd.NewWriter(nil); err != nil {
		panic(err) // this is impossible
	}
	if zstdDecoder, err = zstd.NewReader(nil); err != nil {
		panic(err) // this is impossible
	}
}

// ZstdCompress compresses src and append it to dst.
func ZstdCompress(src, dst []byte) []byte {
	return zstdEncoder.EncodeAll(src, dst)
}

// ZstdDecompress decompresses input and append it to dst.
func ZstdDecompress(input, dst []byte) ([]byte, error) {
	return zstdDecoder.DecodeAll(input, dst)
}

// ZlibCompress compresses data using zlib.
func ZlibCompress(data []byte) ([]byte, error) {
	buf := &bytes.Buffer{}
	zw := zlib.NewWriter(buf)
	if _, err := zw.Write(data); err != nil {
		return nil, errors.Fmt("failed to compress: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, errors.Fmt("error closing zlib writer: %w", err)
	}
	return buf.Bytes(), nil
}

// ZlibDecompress decompresses data using zlib.
func ZlibDecompress(compressed []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	originalData, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := r.Close(); err != nil {
		return nil, err
	}
	return originalData, nil
}
