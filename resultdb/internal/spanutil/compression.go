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

package spanutil

import (
	"bytes"

	"github.com/klauspost/compress/zstd"

	"go.chromium.org/luci/common/errors"
)

var zstdHeader = []byte("ztd\n")

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

// Compressed instructs ToSpanner and FromSpanner functions to compress the
// content with https://godoc.org/github.com/klauspost/compress/zstd encoding.
type Compressed []byte

// ToSpanner implements Value.
func (c Compressed) ToSpanner() any {
	if len(c) == 0 {
		// Do not store empty bytes.
		return []byte(nil)
	}
	return Compress(c)
}

// SpannerPtr implements Ptr.
func (c *Compressed) SpannerPtr(b *Buffer) any {
	return &b.ByteSlice
}

// FromSpanner implements Ptr.
func (c *Compressed) FromSpanner(b *Buffer) error {
	if len(b.ByteSlice) == 0 {
		// do not set to nil; otherwise we lose the buffer.
		*c = (*c)[:0]
	} else {
		// *c might be pointing to an existing memory buffer.
		// Try to reuse it for decoding.
		var err error
		if *c, err = Decompress(b.ByteSlice, *c); err != nil {
			return err
		}
	}

	return nil
}

// Compress compresses data using zstd.
func Compress(data []byte) []byte {
	out := make([]byte, 0, len(data)/2+len(zstdHeader)) // hope for at least 2x compression
	out = append(out, zstdHeader...)
	return zstdEncoder.EncodeAll(data, out)
}

// Decompress decompresses the src compressed with Compress to dest.
// dest is the buffer for decompressed content, it will be reset to 0 length
// before taking the content.
func Decompress(src, dest []byte) ([]byte, error) {
	if !bytes.HasPrefix(src, zstdHeader) {
		return nil, errors.New("expected ztd header")
	}

	dest, err := zstdDecoder.DecodeAll(src[len(zstdHeader):], dest[:0])
	if err != nil {
		return nil, errors.Fmt("failed to decode from zstd: %w", err)
	}
	return dest, nil
}
