// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package serialize

import (
	"bytes"
)

// Buffer is the interface which corresponds to the subset of *bytes.Buffer
// that this package requires.
type Buffer interface {
	String() string
	Bytes() []byte
	Len() int

	Grow(int)

	Read([]byte) (int, error)
	ReadByte() (byte, error)

	Write([]byte) (int, error)
	WriteByte(c byte) error
	WriteString(s string) (int, error)
}

var _ Buffer = (*bytes.Buffer)(nil)

// InvertibleBuffer is just like Buffer, except that it also has a stateful
// Invert() method, which will cause all reads and writes to/from it to be
// inverted (e.g. every byte XOR 0xFF).
//
// Implementing queries requires manipulating the index entries (e.g.
// synthesizing them, parsing them, etc.). In particular, when you have
// a reverse-sorted field (e.g. high to low instead of low to high), it's
// achieved by having all the bits inverted.
//
// All the serialization formats include delimiter information, which the
// parsers only know to parse non-inverted. If we don't have this buffer, we'd
// basically have to invert every byte in the []byte array when we're trying to
// decode a reverse-ordered field (including the bytes of all fields after the
// one we intend to parse) so that the parser can consume as many bytes as it
// needs (and it only knows the number of bytes it needs as it decodes them).
// This InvertibleBuffer lets that happen on the fly without having to flip the
// whole []byte.
//
// If you know you need it, you'll know it's the right thing. If you're not sure
// then you definitely don't need it!
type InvertibleBuffer interface {
	Buffer
	SetInvert(inverted bool)
}

type invertibleBuffer struct {
	Buffer
	invert bool
}

// Invertible returns an InvertibleBuffer based on the Buffer.
func Invertible(b Buffer) InvertibleBuffer {
	return &invertibleBuffer{b, false}
}

func (ib *invertibleBuffer) Read(bs []byte) (int, error) {
	n, err := ib.Buffer.Read(bs)
	if ib.invert {
		for i, b := range bs {
			bs[i] = b ^ 0xFF
		}
	}
	return n, err
}

func (ib *invertibleBuffer) WriteString(s string) (int, error) {
	if ib.invert {
		ib.Grow(len(s))
		for i := 0; i < len(s); i++ {
			if err := ib.Buffer.WriteByte(s[i] ^ 0xFF); err != nil {
				return i, err
			}
		}
		return len(s), nil
	}
	return ib.Buffer.WriteString(s)
}

func (ib *invertibleBuffer) Write(bs []byte) (int, error) {
	if ib.invert {
		ib.Grow(len(bs))
		for i, b := range bs {
			if err := ib.Buffer.WriteByte(b ^ 0xFF); err != nil {
				return i, err
			}
		}
		return len(bs), nil
	}
	return ib.Buffer.Write(bs)
}

func (ib *invertibleBuffer) WriteByte(b byte) error {
	if ib.invert {
		b = b ^ 0xFF
	}
	return ib.Buffer.WriteByte(b)
}

func (ib *invertibleBuffer) ReadByte() (byte, error) {
	ret, err := ib.Buffer.ReadByte()
	if ib.invert {
		ret = ret ^ 0xFF
	}
	return ret, err
}

func (ib *invertibleBuffer) SetInvert(inverted bool) {
	ib.invert = inverted
}
