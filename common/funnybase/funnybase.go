// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package funnybase provides a varint-like signed integer encoding scheme which
// has the property that the encoded integers can be compared with unordered
// byte comparisons and they will sort correctly.
//
// It's less efficient on average than varint for small numbers (it has
// a minimum encoded size of 2 bytes), but is more efficient for large numbers
// (it has a maximum encoded size of 9 bytes for a 64 bit int).
//
// The scheme works like:
//   * given an 2's compliment value V
//   * extract the sign (S) and magnitude (M) of V
//   * Find the position of the highest bit (P), minus 1.
//   * write (bits):
//     * SPPPPPPP MMMMMMMM MM000000
//     * S is 1
//     * P's are the log2(M)-1
//     * M's are the magnitude of V
//     * 0's are padding
//   * Additionally, if the number is negative, invert the bits of all the bytes
//     (e.g. XOR 0xFF). This makes the sign bit S 0 for negative numbers, and
//     makes the ordering of the numbers correct when compared bytewise.
package funnybase

import (
	"bytes"
	"errors"
	"io"
	"math"
)

// MaxFunnyBaseLenN is the maximum length of a funnybase-encoded N-bit integer.
const (
	MaxFunnyBaseLen16 = 3
	MaxFunnyBaseLen32 = 5
	MaxFunnyBaseLen64 = 9
)

// ErrOutOfBounds is panic'd when using Put/PutUint with a buffer that's
// not large enough to hold the value being encoded.
var ErrOutOfBounds = errors.New("funnybase: buffer was too small")

// ErrOverflow is returned when reading an number which is too large for the
// destination type (either uint64 or int64)
var ErrOverflow = errors.New("funnybase: varint overflows")

// ErrUnderflow is returned when reading an number which is too small for the
// destination type (either uint64 or int64)
var ErrUnderflow = errors.New("funnybase: uvarint underflows")

var paddingMasks = [...]uint64{
	0xFFFFFFFF00000000,
	0xFFFF0000,
	0xFF00,
	0xF0,
	0xC,
	0x2,
	0x1,
}

// Calculate the log2 of the unsigned value v.
//
// This is used to find the position of the highest-set bit in v.
//
// from https://graphics.stanford.edu/~seander/bithacks.html#IntegerLog
// 32 bit implementation extended to 64 bits
func uint64Log2(v uint64) uint {
	log := uint(0)
	for i, m := range paddingMasks {
		if v&m != 0 {
			shift := uint(1<<uint(len(paddingMasks)-2)) >> uint(i)
			v >>= shift
			log |= shift
		}
	}
	return log + 1
}

// Put encodes an int64 into buf and returns the number of bytes written.
// If the buffer's length is too small, Put will panic.
func Put(buf []byte, val int64) uint {
	r, err := write(bytes.NewBuffer(buf[:0]), val, uint(len(buf)))
	if err != nil {
		panic(err)
	}
	return r
}

// Write val as a funnybase to the ByteWriter. Will only return an error if
// the underlying ByteWriter returns an error.
func Write(w io.ByteWriter, val int64) error {
	_, err := write(w, val, MaxFunnyBaseLen64)
	return err
}

func write(w io.ByteWriter, val int64, byteLimit uint) (uint, error) {
	var inv byte
	if val < 0 {
		inv = 0xff
	}
	mag := uint64(val)
	if inv != 0 {
		mag = -mag
	}
	return writeSignMag(w, mag, inv, byteLimit)
}

// PutUint encodes an int64 into buf and returns the number of bytes written.
// If the buffer's length is too small, Put will panic.
func PutUint(buf []byte, mag uint64) uint {
	r, err := writeSignMag(bytes.NewBuffer(buf[:0]), mag, 0, uint(len(buf)))
	if err != nil {
		panic(err)
	}
	return r
}

// WriteUint writes mag to the ByteWriter. Will only return an error if the
// underlying ByteWriter returns an error.
func WriteUint(w io.ByteWriter, mag uint64) error {
	_, err := writeSignMag(w, mag, 0, MaxFunnyBaseLen64)
	return err
}

// Get decodes a funnybase-encoded number from a byte slice. Returns
// the decoded value as well as the number of bytes read. If an error
// occurs, the value is 0, and n will be:
//
//  n == 0: buf too small
//  n == -1: value overflows int64
//  n == -2: value underflows int64
func Get(b []byte) (r int64, n int) {
	buf := bytes.NewBuffer(b)
	r, err := Read(buf)
	switch err {
	case ErrOverflow:
		return 0, -1
	case ErrUnderflow:
		return 0, -2
	case io.EOF:
		return 0, 0
	}
	n = len(b) - buf.Len()
	return
}

// GetUint decodes a funnybase-encoded number from a byte slice. Returns
// the decoded value as well as the number of bytes read. If an error
// occurs, the value is 0, and n will be:
//
//  n == 0: buf too small
//  n == -1: value overflows uint64
//  n == -2: value underflows uint64
func GetUint(b []byte) (mag uint64, n int) {
	buf := bytes.NewBuffer(b)
	mag, err := ReadUint(buf)
	switch err {
	case ErrOverflow:
		return 0, -1
	case ErrUnderflow:
		return 0, -2
	case io.EOF:
		return 0, 0
	}
	n = len(b) - buf.Len()
	return
}

// Read decodes a funnybase-encoded number from a ByteReader. It
// returns err{Over,Under}flow if the number is out of bounds. It may also
// return an error if the ByteReader returns an error.
func Read(r io.ByteReader) (int64, error) {
	pos, sigs, mag, err := readSignMag(r)
	if err != nil {
		return 0, err
	}
	if pos {
		if sigs > 63 {
			return 0, ErrOverflow
		}
		return int64(mag), nil
	}
	if mag > uint64(-math.MinInt64) {
		return 0, ErrUnderflow
	}
	return int64(-mag), nil
}

// ReadUint decodes a funnybase-encoded positive number from a ByteReader.  It
// returns err{Over,Under}flow if the number is out of bounds. It may also
// return an error if the ByteReader returns an error.
func ReadUint(r io.ByteReader) (uint64, error) {
	pos, _, mag, err := readSignMag(r)
	if err != nil {
		return 0, err
	}
	if !pos {
		return 0, ErrUnderflow
	}
	return mag, err
}

func writeSignMag(w io.ByteWriter, mag uint64, inv byte, byteLimit uint) (i uint, err error) {
	sigs := uint64Log2(mag)

	wb := func(b byte) error {
		i++
		if i > byteLimit {
			return ErrOutOfBounds
		}
		return w.WriteByte(b)
	}

	if err = wb(byte(0x80|(sigs-1)) ^ inv); err != nil {
		return
	}

	for sigs > 8 {
		sigs -= 8

		if err = wb(byte(mag>>sigs) ^ inv); err != nil {
			return
		}
	}
	if sigs != 0 {
		if err = wb(byte(mag<<(8-sigs)) ^ inv); err != nil {
			return
		}
	}

	return
}

func readSignMag(r io.ByteReader) (positive bool, sigs uint, mag uint64, err error) {
	var inv byte

	b0, err := r.ReadByte()
	if err != nil {
		return
	}
	positive = true
	if b0&0x80 == 0 {
		positive = false
		inv = 0xff
	}

	sigs = uint((b0^inv)&0x7f) + 1
	if sigs > 64 {
		err = ErrOverflow
		return
	}

	n := int((sigs+7)>>3) + 1

	var b byte
	shift := uint(64 - 8)
	for i := 1; i < n; i++ {
		b, err = r.ReadByte()
		if err != nil {
			return
		}
		mag |= uint64(b^inv) << shift
		shift -= 8
	}
	mag >>= 64 - sigs

	return
}
