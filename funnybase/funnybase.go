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

// MaxFunnyBaseLenN is the maximum length of a funnybase-encoded N-bit integer.
const (
	MaxFunnyBaseLen16 = 3
	MaxFunnyBaseLen32 = 5
	MaxFunnyBaseLen64 = 9
)

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
// If the buffer is too small, Put will panic.
func Put(buf []byte, val int64) uint {
	var inv byte
	if val < 0 {
		inv = 0xff
	}
	mag := uint64(val)
	if inv != 0 {
		mag = -mag
	}

	sigs := uint64Log2(mag)

	i := uint(0)
	buf[i] = byte(0x80|(sigs-1)) ^ inv
	for sigs > 8 {
		sigs -= 8
		i++
		buf[i] = byte(mag>>sigs) ^ inv
	}
	if sigs != 0 {
		i++
		buf[i] = byte(mag<<(8-sigs)) ^ inv
	}

	return i + 1
}

// Get decodes a funnybase-encoded number from a byte slice. Returns
// the decoded value as well as the number of bytes read. If an error
// occurs, the value is 0, and n will be:
//
//  n == 0: buf too small
//  n == -1: value overflows int64
func Get(buf []byte) (r int64, n int) {
	var inv byte
	if len(buf) < 1 {
		return 0, 0
	}
	if buf[0]&0x80 == 0 {
		inv = 0xff
	}
	sigs := uint((buf[0]^inv)&0x7f) + 1
	if sigs > 64 {
		return 0, -1
	}

	mag := uint64(0)
	n = int((sigs+7)>>3) + 1
	if len(buf) < n {
		return 0, 0
	}
	shift := uint(64 - 8)
	for i := 1; i < n; i++ {
		mag |= uint64(buf[i]^inv) << shift
		shift -= 8
	}
	mag >>= 64 - sigs

	r = int64(mag)
	if inv != 0 {
		r = -r
	}

	return
}
