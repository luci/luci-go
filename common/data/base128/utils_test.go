// Copyright 2023 The LUCI Authors.
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

package base128

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/bits"
	"testing"
	"testing/quick"

	"github.com/google/go-cmp/cmp"
)

// TestDecodedLen tests that the decoded length of an encoded string is correct
// in a few hand-verified cases.
func TestDecodedLen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dataLen    int
		encodedLen int
	}{
		{
			dataLen:    0,
			encodedLen: 0,
		},
		{
			dataLen:    1,
			encodedLen: 2,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(fmt.Sprintf("%d --> %d", tt.dataLen, tt.encodedLen), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.encodedLen, EncodedLen(tt.dataLen)); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.dataLen, DecodedLen(tt.encodedLen)); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestDecodedLenRoundTrip is a property-based test that tests that taking a message of
// a given length, computing its encoded length, and then computing its decoded length,
// round-trips.
//
// This round-trip property should hold even in cases where the length is invalid
// (congruent to 1 mod 8 or negative).
func TestDecodedLenRoundTrip(t *testing.T) {
	t.Parallel()

	// Choose bounds to avoid annoying wraparound behvaior.
	// If you use a string that big, then all bets are off.
	const maxInt = 1 << 60
	const minInt = -1 * (1 << 30)

	roundTrip := func(input int) bool {
		if input > maxInt {
			return true
		}
		if input < minInt {
			return true
		}
		return input == DecodedLen(EncodedLen(input))
	}

	if err := quick.Check(roundTrip, nil); err != nil {
		t.Error(err)
	}
}

// TestEncode tests encoding an input byte array using the base128 encoding
// in a few hand-verified cases.
//
// These examples were constructed by manually putting the bits in the input
// into the low seven bits of each output byte and then padding the rest with
// zeroes to reach an integer number of bytes.
//
// For example,
//
//	0b_1234_5678
//
// would map to
//
//	0b_0123_4567 0b_0800_0000
//
// In the above example, 0 is a real zero and 1-8 are "variables" of sorts
// representing data positions in an input byte. The above example illustrates
// how the bits of data are spread over the output.
func TestEncode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in  []byte
		out []byte
		ok  bool
	}{
		{
			in:  []byte{},
			out: []byte{},
			ok:  true,
		},
		{
			in:  []byte{0b_1111_1111},
			out: []byte{0b_0111_1111, 0b_0100_0000},
			ok:  true,
		},
		{
			in:  []byte{0b_1111_1110},
			out: []byte{0b_0111_1111, 0b_0000_0000},
			ok:  true,
		},
		{
			in:  []byte{0b_1011_0011},
			out: []byte{0b_0101_1001, 0b_0100_0000},
			ok:  true,
		},
		{
			in:  []byte{0b_1110_1110, 0b_1011_0011},
			out: []byte{0b_0111_0111, 0b_0010_1100, 0b_0110_0000},
			ok:  true,
		},
		{
			in:  []byte{0b_1110_1110, 0b_1011_0011, 0b_1010_1100},
			out: []byte{0b_0111_0111, 0b_0010_1100, 0b_0111_0101, 0b_0100_0000},
			ok:  true,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(hex.EncodeToString(tt.in), func(t *testing.T) {
			t.Parallel()
			out := make([]byte, EncodedLen(len(tt.in)))
			_, err := encode(out, tt.in)
			switch {
			case err == nil && !tt.ok:
				t.Error("error was unexpectedly nil")
			case err != nil && tt.ok:
				t.Errorf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(tt.out, out); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestWriteByte tests the inner loop of encode in a few hand-verified cases.
//
// writeByte(input, offset, bitOffset, val) writes the data byte val to
// input[offset] and input[offset+1] by splitting its bits between the
// low seven bits of those two bytes.
//
// There are seven ways to do this, depending on the bit position where you
// start writing.
func TestWriteByte(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     []byte
		offset    int
		bitOffset int
		val       byte
		output    []byte
	}{
		{
			name:      "FF offset 1",
			input:     []byte{0, 0},
			offset:    0,
			bitOffset: 1,
			val:       0b_1111_1111,
			output:    []byte{0b_0111_1111, 0b_0100_0000},
		},
		{
			name:      "FF offset 2",
			input:     []byte{0, 0},
			offset:    0,
			bitOffset: 2,
			val:       0b_1111_1111,
			output:    []byte{0b_0011_1111, 0b_0110_0000},
		},
		{
			name:      "FF offset 3",
			input:     []byte{0, 0},
			offset:    0,
			bitOffset: 3,
			val:       0b_1111_1111,
			output:    []byte{0b_0001_1111, 0b_0111_0000},
		},
		{
			name:      "FF offset 4",
			input:     []byte{0, 0},
			offset:    0,
			bitOffset: 4,
			val:       0b_1111_1111,
			output:    []byte{0b_0000_1111, 0b_0111_1000},
		},
		{
			name:      "FF offset 5",
			input:     []byte{0, 0},
			offset:    0,
			bitOffset: 5,
			val:       0b_1111_1111,
			output:    []byte{0b_0000_0111, 0b_0111_1100},
		},
		{
			name:      "FF offset 6",
			input:     []byte{0, 0},
			offset:    0,
			bitOffset: 6,
			val:       0b_1111_1111,
			output:    []byte{0b_0000_0011, 0b_0111_1110},
		},
		{
			name:      "FF offset 7",
			input:     []byte{0, 0},
			offset:    0,
			bitOffset: 7,
			val:       0b_1111_1111,
			output:    []byte{0b_0000_0001, 0b_0111_1111},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := tt.input[:]
			writeByte(actual, tt.offset, tt.bitOffset, tt.val)
			if diff := cmp.Diff(tt.output, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestEncodeValidity tests that the output of encode always has the correct length
// and never has the high bit set.
func TestEncodeValidity(t *testing.T) {
	t.Parallel()

	checker := func(in []byte) bool {
		out := make([]byte, EncodedLen(len(in)))
		_, err := encode(out, in)
		if err != nil {
			panic(err)
		}
		if err := Validate(out); err != nil {
			return false
		}
		if err := ValidateLength(len(out)); err != nil {
			return false
		}
		return true
	}

	if err := quick.Check(checker, nil); err != nil {
		t.Error(err)
	}
}

// TestEncodePreservesOnes tests that the output string always has the
// same number of 1s in it as the input string.
//
// In the output string, the high bit of each byte is always zero and
// there may be trailing zeroes. Artificial ones, however, are never
// inserted. This means that the number of ones in the message should
// be preserved by the encoding.
func TestEncodePreservesOnes(t *testing.T) {
	t.Parallel()

	checker := func(in []byte) bool {
		out := make([]byte, EncodedLen(len(in)))
		_, err := encode(out, in)
		if err != nil {
			panic(err)
		}
		return countOnes(in) == countOnes(out)
	}

	if err := quick.Check(checker, nil); err != nil {
		t.Error(err)
	}
}

// countOnes counts the number of ones set in a binary string. This is an invariant of encoding.
func countOnes(input []byte) int {
	tally := 0
	for _, val := range input {
		tally += bits.OnesCount8(val)
	}
	return tally
}

// TestWriteEncodedByte tests reading a byte from an encoded string and
// writing it to a decoded string. This is the inner loop of the decode function.
//
// writeEncodedByte(dst, dstIndex, src, offset, bitOffset) works by reading
// the types src[offset] and, if necessary, src[offset+1].
// the bitOffset is the number of bits from the beginning of src[offset] to start reading
// from.
// We then read the next eight bits into the destination byte dst[dstIndex].
//
// For example,
//
// 0b_0123_4567 0b_0800_0000
//
// maps to
//
// 0b_1234_5678
//
// when bitOffset is 1. In the above example, 0 is a literal zero and 1-8 are
// variables indicating bits of data. The first bit of every byte in an encoded string
// is always zero, as are the trailing bits in the last byte of an encoded string that
// don't encode anything.
func TestWriteEncodedByte(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     []byte
		dstIndex  int
		offset    int
		bitOffset int
		output    []byte
	}{
		{
			name:      "FF offset 1",
			input:     []byte{0b_0111_1111, 0b_0100_0000},
			dstIndex:  0,
			offset:    0,
			bitOffset: 1,
			output:    []byte{0b_1111_1111},
		},
		{
			name:      "FF offset 7",
			input:     []byte{0b_0000_0001, 0b_0111_1111},
			dstIndex:  0,
			offset:    0,
			bitOffset: 7,
			output:    []byte{0b_1111_1111},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := make([]byte, len(tt.output))
			writeEncodedByte(actual, tt.dstIndex, tt.input, tt.offset, tt.bitOffset)
			if diff := cmp.Diff(tt.output, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestDecode tests decoding for a few hand-verified examples.
//
// Examples are produced by taking an encoding string, looking at the bottom seven bits of each
// byte, and then reorganizing those into a decoded form.
func TestDecode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		input  []byte
		output []byte
		ok     bool
	}{
		{
			name:   "FF",
			input:  []byte{0b_0111_1111, 0b_0100_0000},
			output: []byte{0b_1111_1111},
			ok:     true,
		},
		{
			name:   "alternating bits -- encoded length 3",
			input:  []byte{0b_0101_0101, 0b_0101_0101, 0b_0100_0000},
			output: []byte{0b_1010_1011, 0b_0101_0110},
			ok:     true,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := make([]byte, len(tt.output))
			_, err := decode(actual, tt.input)
			switch {
			case err == nil && !tt.ok:
				t.Error("err is unexpectedly nil")
			case err != nil && tt.ok:
				t.Errorf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(tt.output, actual); diff != "" {
				t.Errorf("unexpected diff: %s", diff)
			}
		})
	}
}

// TestEncodeDecodeRoundTripDeterministic is a deterministic test (rather than
// a property-based test) that tests that data strings round-trip.
func TestEncodeDecodeRoundTripDeterministic(t *testing.T) {
	t.Parallel()

	cases := []struct {
		msg string
	}{
		{
			msg: "",
		},
		{
			msg: "hi",
		},
		{
			msg: "AAAAAAAAAA",
		},
		{
			msg: "\x00",
		},
		{
			msg: "\x00\x00",
		},
		{
			msg: "1356825f-a738-4c96-8254-617923483d13",
		},
		{
			msg: "0ffe6517-9a7c-4b7e-8f67-2f7e803224bc",
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(hex.EncodeToString([]byte(tt.msg)), func(t *testing.T) {
			t.Parallel()
			encoded := make([]byte, EncodedLen(len(tt.msg)))
			_, err := encode(encoded, []byte(tt.msg))
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			roundtripped := make([]byte, len(tt.msg))
			_, err = decode(roundtripped, encoded)
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(tt.msg, string(roundtripped)); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestEncodeDecodeRoundTrip is a property-based test that checks whether
// encoding and decoding a randomly generated byte array produces the same byte array.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	roundTrip := func(input []byte) bool {
		encoded := make([]byte, EncodedLen(len(input)))
		_, err := encode(encoded, input)
		if err != nil {
			panic(err)
		}
		roundtripped := make([]byte, len(input))
		_, err = decode(roundtripped, encoded)
		if err != nil {
			panic(err)
		}
		return bytes.Equal(input, roundtripped)
	}

	if err := quick.Check(roundTrip, nil); err != nil {
		t.Error(err)
	}
}
