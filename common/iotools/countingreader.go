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

// CountingReader is an io.Reader that counts the number of bytes that are read.
type CountingReader struct {
	io.Reader // The underlying io.Reader.

	Count int64
}

var _ io.Reader = (*CountingReader)(nil)

// Read implements io.Reader.
func (c *CountingReader) Read(buf []byte) (int, error) {
	amount, err := c.Reader.Read(buf)
	c.Count += int64(amount)
	return amount, err
}

// ReadByte implements io.ByteReader.
func (c *CountingReader) ReadByte() (byte, error) {
	// If our underlying reader is a ByteReader, use its ReadByte directly.
	if br, ok := c.Reader.(io.ByteReader); ok {
		b, err := br.ReadByte()
		if err == nil {
			c.Count++
		}
		return b, err
	}

	data := []byte{0}
	amount, err := c.Reader.Read(data)
	if amount != 0 {
		c.Count += int64(amount)
		return data[0], err
	}
	return 0, err
}
