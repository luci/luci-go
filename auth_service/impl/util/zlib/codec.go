// Copyright 2024 The LUCI Authors.
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

// Package zlib contains functions for zlib encoding and decoding.
package zlib

import (
	"bytes"
	"compress/zlib"
	"io"
)

func Compress(input []byte) ([]byte, error) {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	if _, err := w.Write(input); err != nil {
		// Error writing; close the writer before returning.
		_ = w.Close()
		return nil, err
	}

	if err := w.Close(); err != nil {
		// Error closing writer.
		return nil, err
	}

	return b.Bytes(), nil
}

func Decompress(input []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewBuffer(input))
	if err != nil {
		// Error creating reader.
		return nil, err
	}

	w := bytes.NewBuffer([]byte{})
	if _, err := io.Copy(w, r); err != nil {
		// Error copying from reader; close the reader before returning.
		_ = r.Close()
		return nil, err
	}

	if err := r.Close(); err != nil {
		// Error closing the reader.
		return nil, err
	}

	return w.Bytes(), nil
}
