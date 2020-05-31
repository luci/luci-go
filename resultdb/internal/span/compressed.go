// Copyright 2019 The LUCI Authors.
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

package span

import (
	"bytes"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/klauspost/compress/zstd"
	"go.chromium.org/luci/common/errors"
)

// Globally shared zstd encoder and decoder. We use only their EncodeAll and
// DecodeAll methods which are allowed to be used concurrently. Internally, both
// the encode and the decoder have worker pools (limited by GOMAXPROCS) and each
// concurrent EncodeAll/DecodeAll call temporary consumes one worker (so overall
// we do not run more jobs that we have cores for).
var (
	zstdHeader  = []byte("ztd\n")
	zstdEncoder *zstd.Encoder
	zstdDecoder *zstd.Decoder
)

// Compressed instructs ToSpanner and FromSpanner functions to compress the
// content with https://godoc.org/github.com/klauspost/compress/zstd encoding.
type Compressed []byte

// ToSpanner implements span.Value.
func (c Compressed) ToSpanner() interface{} {
	if len(c) == 0 {
		// Do not store empty bytes.
		return []byte(nil)
	}
	return compress(c)
}

// SpannerPtr implements span.Ptr.
func (c *Compressed) SpannerPtr(b *Buffer) interface{} {
	return &b.byteSlice
}

// FromSpanner implements span.Ptr.
func (c *Compressed) FromSpanner(b *Buffer) error {
	if len(b.byteSlice) == 0 {
		// do not set to nil; otherwise we lose the buffer.
		*c = (*c)[:0]
	} else {
		// *c might be pointing to an existing memory buffer.
		// Try to reuse it for decoding.
		var err error
		if *c, err = decompress(b.byteSlice, *c); err != nil {
			return err
		}
	}

	return nil
}

// CompressedProto is like Compressed, but for protobuf messages.
//
// Although not strictly necessary, FromSpanner expects a *CompressedProto,
// for consistency.
//
// Note that `type CompressedProto proto.Message` and
// `type CompressedProto interface{proto.Message}` do not work well with
// type-switches.
type CompressedProto struct{ Message proto.Message }

// SpannerPtr implements span.Ptr.
func (c CompressedProto) SpannerPtr(b *Buffer) interface{} {
	return &b.byteSlice
}

// FromSpanner implements span.Ptr.
func (c CompressedProto) FromSpanner(b *Buffer) error {
	c.Message.Reset()
	if len(b.byteSlice) != 0 {
		var err error
		if b.byteSlice2, err = decompress(b.byteSlice, b.byteSlice2); err != nil {
			return err
		}
		if err := proto.Unmarshal(b.byteSlice2, c.Message); err != nil {
			return err
		}
	}

	return nil
}

// ToSpanner implements span.Value.
func (c CompressedProto) ToSpanner() interface{} {
	if isMessageNil(c.Message) {
		// Do not store empty messages.
		return []byte(nil)
	}

	// This might be optimized by reusing the memory buffer.
	raw, err := proto.Marshal(c.Message)
	if err != nil {
		panic(err)
	}
	return compress(raw)
}

func compress(data []byte) []byte {
	out := make([]byte, 0, len(data)/2+len(zstdHeader)) // hope for at least 2x compression
	out = append(out, zstdHeader...)
	return zstdEncoder.EncodeAll(data, out)
}

func decompress(src, dest []byte) ([]byte, error) {
	if !bytes.HasPrefix(src, zstdHeader) {
		return nil, errors.Reason("expected ztd header").Err()
	}

	dest, err := zstdDecoder.DecodeAll(src[len(zstdHeader):], dest[:0])
	if err != nil {
		return nil, errors.Annotate(err, "failed to decode from zstd").Err()
	}
	return dest, nil
}

func isMessageNil(m proto.Message) bool {
	return reflect.ValueOf(m).IsNil()
}
