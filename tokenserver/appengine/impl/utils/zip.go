// Copyright 2016 The LUCI Authors.
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

package utils

import (
	"bytes"
	"compress/zlib"
	"io"
)

// ZlibCompress zips a blob using zlib.
func ZlibCompress(in []byte) ([]byte, error) {
	out := bytes.Buffer{}
	w := zlib.NewWriter(&out)
	_, writeErr := w.Write(in)
	closeErr := w.Close()
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return out.Bytes(), nil
}

// ZlibDecompress unzips a blob using zlib.
func ZlibDecompress(in []byte) ([]byte, error) {
	out := bytes.Buffer{}
	r, err := zlib.NewReader(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	_, readErr := io.Copy(&out, r)
	closeErr := r.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return out.Bytes(), nil
}
