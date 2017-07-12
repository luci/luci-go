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

package cmpbin

import (
	"encoding/binary"
	"io"
	"math"
)

// WriteFloat64 writes a memcmp-sortable float to buf. It returns the number
// of bytes written (8, unless an error occurs), and any write error
// encountered.
func WriteFloat64(buf io.Writer, v float64) (n int, err error) {
	bits := math.Float64bits(v)
	bits = bits ^ (-(bits >> 63) | (1 << 63))
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, bits)
	return buf.Write(data)
}

// ReadFloat64 reads a memcmp-sortable float from buf (as written by
// WriteFloat64). It also returns the number of bytes read (8, unless an error
// occurs), and any read error encountered.
func ReadFloat64(buf io.Reader) (ret float64, n int, err error) {
	// byte-ordered floats http://stereopsis.com/radix.html
	data := make([]byte, 8)
	if n, err = buf.Read(data); err != nil {
		return
	}
	bits := binary.BigEndian.Uint64(data)
	ret = math.Float64frombits(bits ^ (((bits >> 63) - 1) | (1 << 63)))
	return
}
