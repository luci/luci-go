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
	"fmt"
	"io"
	"math"
	"math/rand"
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testCase struct {
	expect []byte
	val    int64
}

type testCaseSlice []testCase

func (t testCaseSlice) Len() int           { return len(t) }
func (t testCaseSlice) Less(i, j int) bool { return t[i].val < t[j].val }
func (t testCaseSlice) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }

var cases = testCaseSlice{
	{[]byte{0b01000000, 0b01111111, 0b11111111, 0b11111111, 0b11111111, 0xff, 0xff, 0xff, 0b11111111}, -math.MaxInt64 - 1},
	{[]byte{0b01000001, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0, 0, 0, 0b00000001}, -math.MaxInt64},
	{[]byte{0b01000001, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0, 0, 0, 0b00000011}, -math.MaxInt64 + 1},
	{[]byte{0b01100000, 0b01111111, 0b11111111, 0b11111111, 0b11111111}, -math.MaxInt32 - 1},
	{[]byte{0b01100001, 0b00000000, 0b00000000, 0b00000000, 0b00000001}, -math.MaxInt32},
	{[]byte{0b01100001, 0b00000000, 0b00000000, 0b00000000, 0b00000011}, -math.MaxInt32 + 1},
	{[]byte{0b01110000, 0b01111111, 0b11111111}, -math.MaxInt16 - 1},
	{[]byte{0b01111000, 0b01111110}, -129},
	{[]byte{0b01111000, 0b01111111}, -128},
	{[]byte{0b01111001, 0b00000001}, -127},
	{[]byte{0b01111001, 0b01111101}, -65},
	{[]byte{0b01111001, 0b01111111}, -64},
	{[]byte{0b01111010, 0b00000011}, -63},
	{[]byte{0b01111101, 0b01011111}, -5},
	{[]byte{0b01111110, 0b00111111}, -3},
	{[]byte{0b01111111, 0b01111111}, -1},
	{[]byte{0b10000000, 0b00000000}, 0},
	{[]byte{0b10000010, 0b10100000}, 5},
	{[]byte{0b10000100, 0b10001000}, 17},
	{[]byte{0b10000101, 0b11111100}, 63},
	{[]byte{0b10000110, 0b10000000}, 64},
	{[]byte{0b10000110, 0b10000010}, 65},
	{[]byte{0b10000111, 0b10000000}, 128},
	{[]byte{0b10011110, 0b11111111, 0xff, 0xff, 0b11111110}, math.MaxInt32},
	{[]byte{0b10011111, 0b10000000, 0, 0, 0}, math.MaxInt32 + 1},
	{[]byte{0b10111110, 0b11111111, 0xff, 0xff, 0b11111111, 0xff, 0xff, 0xff, 0b11111110}, math.MaxInt64},
}

func TestWrite(t *testing.T) {
	t.Parallel()
	ftt.Run("WriteFuncs", t, func(t *ftt.Test) {
		for _, c := range cases {
			c := c
			t.Run(fmt.Sprintf("%d -> % x", c.val, c.expect), func(t *ftt.Test) {
				t.Run("Write", func(t *ftt.Test) {
					buf := &bytes.Buffer{}
					n, err := WriteInt(buf, c.val)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, n, should.Equal(len(c.expect)))
					assert.Loosely(t, buf.Bytes(), should.Match(c.expect))
				})

				if c.val >= 0 {
					t.Run("WriteUint", func(t *ftt.Test) {
						buf := &bytes.Buffer{}
						n, err := WriteUint(buf, uint64(c.val))
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, n, should.Equal(len(c.expect)))
						assert.Loosely(t, buf.Bytes(), should.Match(c.expect))
					})
				}
			})
		}
	})
}

func TestRead(t *testing.T) {
	ftt.Run("Read", t, func(t *ftt.Test) {
		for _, c := range cases {
			c := c
			t.Run(fmt.Sprintf("% x -> %d", c.expect, c.val), func(t *ftt.Test) {
				buf := bytes.NewBuffer(c.expect)
				v, n, err := ReadInt(buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, n, should.Equal(len(c.expect)))
				assert.Loosely(t, v, should.Equal(c.val))

				if c.val >= 0 {
					buf := bytes.NewBuffer(c.expect)
					v, n, err := ReadUint(buf)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, n, should.Equal(len(c.expect)))
					assert.Loosely(t, v, should.Equal(uint64(c.val)))
				}
			})
		}
	})
}

func TestSort(t *testing.T) {
	num := randomTestSize
	num += len(cases)
	randomCases := make(testCaseSlice, num)

	rcSub := randomCases[copy(randomCases, cases):]
	r := rand.New(rand.NewSource(*seed))
	buf := &bytes.Buffer{}
	for i := range rcSub {
		v := int64(uint64(r.Uint32())<<32 | uint64(r.Uint32()))
		rcSub[i].val = v
		buf.Reset()
		if _, err := WriteInt(buf, v); err != nil {
			panic(err)
		}
		rcSub[i].expect = make([]byte, buf.Len())
		copy(rcSub[i].expect, buf.Bytes())
	}

	sort.Sort(randomCases)

	shouldBeLessThanOrEqual := func(actual any, expected ...any) string {
		a, b := actual.([]byte), expected[0].([]byte)
		if bytes.Compare(a, b) <= 0 {
			return fmt.Sprintf("Expected A <= B (but it wasn't)!\nA: [% x]\nB: [% x]", a, b)
		}
		return ""
	}

	ftt.Run("TestSort", t, func(t *ftt.Test) {
		prev := randomCases[0]
		for _, c := range randomCases[1:] {
			// Actually asserting with the So for every entry in the sorted array will
			// produce 100 green checkmarks on a successful test, which is a bit
			// much :).
			if bytes.Compare(c.expect, prev.expect) < 0 {
				assert.Loosely(t, c.expect, convey.Adapt(shouldBeLessThanOrEqual)(prev.expect))
				break
			}
			prev = c
		}

		// This silly assertion is done so that this test has a green check next to
		// it in the event that it passes. Otherwise convey thinks we skipped the
		// test, which isn't correct.
		assert.Loosely(t, true, should.BeTrue)
	})
}

func TestErrors(t *testing.T) {
	smallerInt64 := []byte{0b01000000, 0b01111111, 0b11111111, 0b11111111, 0b11111111, 0xff, 0xff, 0xff, 0b11111110}

	prettyBigUint64 := []byte{0b10111111, 0b10000000, 0, 0, 0, 0, 0, 0, 0}
	prettyBigUint64Val := uint64(math.MaxInt64 + 1)

	reallyBigUint64 := []byte{0b10111111, 0b11111111, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	reallyBigUint64Val := uint64(math.MaxUint64)
	tests := []struct {
		name string
		buf  []byte

		v   int64
		n   int
		err error

		uv   uint64
		un   int
		uerr error
	}{
		{
			name: "Too big!!",
			buf:  []byte{0b11000000}, // 65 bits!?
			err:  ErrOverflow,
			uerr: ErrOverflow,
		}, {
			name: "Nil buffer",
			err:  io.EOF,
			uerr: io.EOF,
		}, {
			name: "Empty buffer",
			buf:  []byte{},
			err:  io.EOF,
			uerr: io.EOF,
		}, {
			name: "Small buffer",
			buf:  cases[len(cases)-1].expect[:4],
			err:  io.EOF,
			uerr: io.EOF,
		}, {
			name: "Reading a negative number with *Uint",
			buf:  cases[0].expect,
			v:    cases[0].val,
			n:    len(cases[0].expect),

			uerr: ErrUnderflow,
		}, {
			name: "Reading a number smaller than min int64",
			buf:  smallerInt64,
			err:  ErrUnderflow,
			uerr: ErrUnderflow,
		}, {
			name: "Reading a number bigger than int64",
			buf:  prettyBigUint64,
			err:  ErrOverflow,

			uv: prettyBigUint64Val,
			un: len(prettyBigUint64),
		}, {
			name: "Reading MaxUint64",
			buf:  reallyBigUint64,
			err:  ErrOverflow,

			uv: reallyBigUint64Val,
			un: len(reallyBigUint64),
		},
	}

	ftt.Run("Error conditions", t, func(t *ftt.Test) {
		for _, tc := range tests {
			t.Run(tc.name, func(t *ftt.Test) {
				t.Run("Read", func(t *ftt.Test) {
					v, _, err := ReadInt(bytes.NewBuffer(tc.buf))
					assert.Loosely(t, err, should.Equal(tc.err))
					if tc.err == nil {
						assert.Loosely(t, v, should.Equal(tc.v))
					}
				})
				t.Run("ReadUint", func(t *ftt.Test) {
					uv, _, err := ReadUint(bytes.NewBuffer(tc.buf))
					assert.Loosely(t, err, should.Equal(tc.uerr))
					if tc.uerr == nil {
						assert.Loosely(t, uv, should.Equal(tc.uv))
					}
				})
			})
		}
		t.Run("Write Errors", func(t *ftt.Test) {
			// Test each error return location in writeSignMag
			for count := 0; count < 3; count++ {
				fw := &fakeWriter{count}
				_, err := WriteInt(fw, -10000)
				assert.Loosely(t, err.Error(), should.ContainSubstring("nope"))
				assert.Loosely(t, fw.count, should.BeZero)
			}
		})
	})
}
