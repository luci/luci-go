// Copyright 2015 The LUCI Authors.
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

package iotools

import (
	"io"
)

// ByteSliceReader is an io.Reader and io.ByteReader implementation that reads
// and mutates an underlying byte slice.
type ByteSliceReader []byte

var _ interface {
	io.Reader
	io.ByteReader
} = (*ByteSliceReader)(nil)

// Read implements io.Reader.
func (r *ByteSliceReader) Read(buf []byte) (int, error) {
	count := copy(buf, *r)
	*r = (*r)[count:]
	if len(*r) == 0 {
		*r = nil
	}
	if count < len(buf) {
		return count, io.EOF
	}
	return count, nil
}

// ReadByte implements io.ByteReader.
func (r *ByteSliceReader) ReadByte() (byte, error) {
	d := []byte{0}
	_, err := r.Read(d)
	return d[0], err
}
