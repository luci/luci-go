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
	"bytes"
)

// WriteableBytesBuffer is the interface which corresponds to the subset of
// *bytes.Buffer required for writing.
type WriteableBytesBuffer interface {
	ReadableBytesBuffer

	String() string
	Bytes() []byte

	Grow(int)

	Write([]byte) (int, error)
	WriteByte(c byte) error
	WriteString(s string) (int, error)
}

// ReadableBytesBuffer is the interface which corresponds to the subset of
// *bytes.Reader required for reading.
type ReadableBytesBuffer interface {
	Len() int

	Read([]byte) (int, error)
	ReadByte() (byte, error)
}

var (
	_ WriteableBytesBuffer = (*bytes.Buffer)(nil)
	_ ReadableBytesBuffer  = (*bytes.Reader)(nil)
)

// InvertibleBytesBuffer is just like Buffer, except that it also has a stateful
// SetInvert() method, which will cause all reads and writes to/from it to be
// inverted (e.g. every byte XOR 0xFF).
//
// In contexts where you need comparable byte sequences, like datastore queries,
// it requires manipulating the sortable fields (e.g. synthesizing them,
// parsing them, etc.). In particular, when you have a reverse-sorted field
// (e.g. high to low instead of low to high), it's achieved by having all the
// bits inverted.
//
// Serialization formats (like gae/service/datastore.Serialize) can include
// delimiter information, which the parsers only know to parse non-inverted. If
// we don't have this Invertible buffer, we'd basically have to invert every
// byte in the []byte array when we're trying to decode a reverse-ordered field
// (including the bytes of all fields after the one we intend to parse) so that
// the parser can consume as many bytes as it needs (and it only knows the
// number of bytes it needs as it decodes them). This InvertibleBytesBuffer
// lets that happen on the fly without having to flip the whole underlying
// []byte.
//
// If you know you need it, you'll know it's the right thing.
// If you're not sure then you definitely don't need it!
//
// See InvertBytes for doing one-off []byte inversions.
type InvertibleBytesBuffer interface {
	WriteableBytesBuffer
	SetInvert(inverted bool)
}

type invertibleBytesBuffer struct {
	WriteableBytesBuffer
	invert bool
}

// Invertible returns an InvertibleBuffer based on the Buffer.
func Invertible(b WriteableBytesBuffer) InvertibleBytesBuffer {
	return &invertibleBytesBuffer{b, false}
}

func (ib *invertibleBytesBuffer) Read(bs []byte) (int, error) {
	n, err := ib.WriteableBytesBuffer.Read(bs)
	if ib.invert {
		for i, b := range bs {
			bs[i] = b ^ 0xFF
		}
	}
	return n, err
}

func (ib *invertibleBytesBuffer) WriteString(s string) (int, error) {
	if ib.invert {
		ib.Grow(len(s))
		for i := range len(s) {
			if err := ib.WriteableBytesBuffer.WriteByte(s[i] ^ 0xFF); err != nil {
				return i, err
			}
		}
		return len(s), nil
	}
	return ib.WriteableBytesBuffer.WriteString(s)
}

func (ib *invertibleBytesBuffer) Write(bs []byte) (int, error) {
	if ib.invert {
		ib.Grow(len(bs))
		for i, b := range bs {
			if err := ib.WriteableBytesBuffer.WriteByte(b ^ 0xFF); err != nil {
				return i, err
			}
		}
		return len(bs), nil
	}
	return ib.WriteableBytesBuffer.Write(bs)
}

func (ib *invertibleBytesBuffer) WriteByte(b byte) error {
	if ib.invert {
		b = b ^ 0xFF
	}
	return ib.WriteableBytesBuffer.WriteByte(b)
}

func (ib *invertibleBytesBuffer) ReadByte() (byte, error) {
	ret, err := ib.WriteableBytesBuffer.ReadByte()
	if ib.invert {
		ret = ret ^ 0xFF
	}
	return ret, err
}

func (ib *invertibleBytesBuffer) SetInvert(inverted bool) {
	ib.invert = inverted
}
