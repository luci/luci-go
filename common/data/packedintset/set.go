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

// Package packedintset implements a way to store integer sets in compact form.
//
// Integers to be packed must already be sorted. They are converted into
// delta-encoded varints and then zlib-compressed.
//
// This is used primarily in `cas` binary to store sets of file sizes it
// downloads or uploads.
package packedintset

import (
	"bytes"
	"io"

	"github.com/klauspost/compress/zlib"

	"go.chromium.org/luci/common/errors"
)

// Pack returns a deflate'd buffer of delta encoded varints.
//
// Inputs must be sorted in ascending order already.
func Pack(values []int64) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if values[0] < 0 {
		return nil, errors.New("values must be between 0 and 2**63")
	}
	if values[len(values)-1] < 0 {
		return nil, errors.New("values must be between 0 and 2**63")
	}

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	var last int64
	for _, value := range values {
		v := value
		value -= last
		if value < 0 {
			_ = w.Close()
			return nil, errors.New("list must be sorted ascending")
		}
		last = v
		for value > 127 {
			if _, err := w.Write([]byte{byte(1<<7 | value&0x7f)}); err != nil {
				_ = w.Close()
				return nil, errors.Fmt("failed to write: %w", err)
			}
			value >>= 7
		}
		if _, err := w.Write([]byte{byte(value)}); err != nil {
			_ = w.Close()
			return nil, errors.Fmt("failed to write: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, errors.Fmt("failed to close zlib writer: %w", err)
	}

	return b.Bytes(), nil
}

// Unpack decompresses a deflate'd delta encoded list of varints.
func Unpack(data []byte) ([]int64, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var ret []int64
	var value int64
	var base int64 = 1
	var last int64

	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Fmt("failed to get zlib reader: %w", err)
	}

	data, err = io.ReadAll(r)
	if err != nil {
		_ = r.Close()
		return nil, errors.Fmt("failed to read all: %w", err)
	}

	for _, valByte := range data {
		value += int64(valByte&0x7f) * base
		if valByte&0x80 > 0 {
			base <<= 7
			continue
		}
		ret = append(ret, value+last)
		last += value
		value = 0
		base = 1
	}

	if err := r.Close(); err != nil {
		return nil, errors.Fmt("failed to close zlib reader: %w", err)
	}

	return ret, nil
}
