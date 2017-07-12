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

// Package base128 implements base128 encoding and decoding.
//
// Encoded results are UTF8-safe, and will retain the same memcmp sorting
// properties of the input data.
package base128

import "errors"

var (
	// ErrLength is returned from the Decode* methods if the input has an
	// impossible length.
	ErrLength = errors.New("base128: invalid length base128 string")

	// ErrBit is returned from the Decode* methods if the input has a byte with
	// the high-bit set (e.g. 0x80). This will never be the case for strings
	// produced from the Encode* methods in this package.
	ErrBit = errors.New("base128: high bit set in base128 string")
)

// Encode encodes src into EncodedLen(len(src)) bytes of dst. As a convenience,
// it returns the number of bytes written to dst, but this value is always
// EncodedLen(len(src)).
//
// Encode implements base128 encoding.
func Encode(dst, src []byte) int {
	ret := EncodedLen(len(src))
	if len(dst) < ret {
		panic("dst has insufficient length")
	}
	dst = dst[:0]
	whichByte := uint(1)
	bufByte := byte(0)
	for _, v := range src {
		dst = append(dst, bufByte|(v>>whichByte))
		bufByte = (v&(1<<whichByte) - 1) << (7 - whichByte)
		if whichByte == 7 {
			dst = append(dst, bufByte)
			bufByte = 0
			whichByte = 0
		}
		whichByte++
	}
	dst = append(dst, bufByte)
	return ret
}

// Decode decodes src into DecodedLen(len(src)) bytes, returning the actual
// number of bytes written to dst.
//
// If Decode encounters invalid input, it returns an error describing the
// failure.
func Decode(dst, src []byte) (int, error) {
	dLen := DecodedLen(len(src))
	if EncodedLen(dLen) != len(src) {
		return 0, ErrLength
	}
	if len(dst) < dLen {
		panic("dst has insufficient length")
	}
	dst = dst[:0]
	whichByte := uint(1)
	bufByte := byte(0)
	for _, v := range src {
		if (v & 0x80) != 0 {
			return len(dst), ErrBit
		}
		if whichByte > 1 {
			dst = append(dst, bufByte|(v>>(8-whichByte)))
		}
		bufByte = v << whichByte
		if whichByte == 8 {
			whichByte = 0
		}
		whichByte++
	}
	return len(dst), nil
}

// DecodeString returns the bytes represented by the base128 string s.
func DecodeString(s string) ([]byte, error) {
	src := []byte(s)
	dst := make([]byte, DecodedLen(len(src)))
	if _, err := Decode(dst, src); err != nil {
		return nil, err
	}
	return dst, nil
}

// DecodedLen returns the number of bytes `encLen` encoded bytes decodes to.
func DecodedLen(encLen int) int {
	return (encLen * 7 / 8)
}

// EncodedLen returns the number of bytes that `dataLen` bytes will encode to.
func EncodedLen(dataLen int) int {
	return (((dataLen * 8) + 6) / 7)
}

// EncodeToString returns the base128 encoding of src.
func EncodeToString(src []byte) string {
	dst := make([]byte, EncodedLen(len(src)))
	Encode(dst, src)
	return string(dst)
}
