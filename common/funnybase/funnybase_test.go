// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package funnybase

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
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
	{[]byte{b01000000, b01111111, b11111111, b11111111, b11111111, 0xff, 0xff, 0xff, b11111111}, -math.MaxInt64 - 1},
	{[]byte{b01000001, b00000000, b00000000, b00000000, b00000000, 0, 0, 0, b00000001}, -math.MaxInt64},
	{[]byte{b01000001, b00000000, b00000000, b00000000, b00000000, 0, 0, 0, b00000011}, -math.MaxInt64 + 1},
	{[]byte{b01100000, b01111111, b11111111, b11111111, b11111111}, -math.MaxInt32 - 1},
	{[]byte{b01100001, b00000000, b00000000, b00000000, b00000001}, -math.MaxInt32},
	{[]byte{b01100001, b00000000, b00000000, b00000000, b00000011}, -math.MaxInt32 + 1},
	{[]byte{b01110000, b01111111, b11111111}, -math.MaxInt16 - 1},
	{[]byte{b01111000, b01111110}, -129},
	{[]byte{b01111000, b01111111}, -128},
	{[]byte{b01111001, b00000001}, -127},
	{[]byte{b01111001, b01111101}, -65},
	{[]byte{b01111001, b01111111}, -64},
	{[]byte{b01111010, b00000011}, -63},
	{[]byte{b01111101, b01011111}, -5},
	{[]byte{b01111110, b00111111}, -3},
	{[]byte{b01111111, b01111111}, -1},
	{[]byte{b10000000, b00000000}, 0},
	{[]byte{b10000010, b10100000}, 5},
	{[]byte{b10000100, b10001000}, 17},
	{[]byte{b10000101, b11111100}, 63},
	{[]byte{b10000110, b10000000}, 64},
	{[]byte{b10000110, b10000010}, 65},
	{[]byte{b10000111, b10000000}, 128},
	{[]byte{b10011110, b11111111, 0xff, 0xff, b11111110}, math.MaxInt32},
	{[]byte{b10011111, b10000000, 0, 0, 0}, math.MaxInt32 + 1},
	{[]byte{b10111110, b11111111, 0xff, 0xff, b11111111, 0xff, 0xff, 0xff, b11111110}, math.MaxInt64},
}

var seed = flag.Int64("funnybase.seed", 0, "Random seed to use for randomized funnybase tests")

func init() {
	flag.Parse()
	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	fmt.Println("funnybase.seed =", *seed)
}

func TestPut(t *testing.T) {
	Convey("Put", t, func() {
		for _, c := range cases {
			c := c
			Convey(fmt.Sprintf("%d -> % x", c.val, c.expect), func() {
				Convey("Put", func() {
					buf := make([]byte, MaxFunnyBaseLen64)
					n := Put(buf, c.val)
					So(n, ShouldEqual, len(c.expect))
					So(buf[:n], ShouldResemble, c.expect)
				})
				Convey("Write", func() {
					buf := &bytes.Buffer{}
					err := Write(buf, c.val)
					So(err, ShouldBeNil)
					So(buf.Bytes(), ShouldResemble, c.expect)
				})

				if c.val >= 0 {
					Convey("PutUint", func() {
						buf := make([]byte, MaxFunnyBaseLen64)
						n := PutUint(buf, uint64(c.val))
						So(n, ShouldEqual, len(c.expect))
						So(buf[:n], ShouldResemble, c.expect)
					})
					Convey("WriteUint", func() {
						buf := &bytes.Buffer{}
						err := WriteUint(buf, uint64(c.val))
						So(err, ShouldBeNil)
						So(buf.Bytes(), ShouldResemble, c.expect)
					})
				}
			})
		}
	})
}

func TestGet(t *testing.T) {
	Convey("Get", t, func() {
		for _, c := range cases {
			c := c
			Convey(fmt.Sprintf("% x -> %d", c.expect, c.val), func() {
				v, n := Get(c.expect)
				So(n, ShouldEqual, len(c.expect))
				So(v, ShouldEqual, c.val)

				if c.val >= 0 {
					v, n := GetUint(c.expect)
					So(n, ShouldEqual, len(c.expect))
					So(v, ShouldEqual, c.val)
				}
			})
		}
	})
}

func TestRead(t *testing.T) {
	Convey("Read", t, func() {
		for _, c := range cases {
			c := c
			Convey(fmt.Sprintf("% x -> %d", c.expect, c.val), func() {
				buf := bytes.NewBuffer(c.expect)
				v, err := Read(buf)
				So(err, ShouldBeNil)
				So(v, ShouldEqual, c.val)

				if c.val >= 0 {
					buf := bytes.NewBuffer(c.expect)
					v, err := ReadUint(buf)
					So(err, ShouldBeNil)
					So(v, ShouldEqual, c.val)
				}
			})
		}
	})
}

func TestSort(t *testing.T) {
	num := 20000000
	if testing.Short() {
		num = 100000
	}
	num += len(cases)
	randomCases := make(testCaseSlice, num)

	rcSub := randomCases[copy(randomCases, cases):]
	r := rand.New(rand.NewSource(*seed))
	for i := range rcSub {
		v := int64(uint64(r.Uint32())<<32 | uint64(r.Uint32()))
		rcSub[i].val = v
		buf := make([]byte, MaxFunnyBaseLen64)
		rcSub[i].expect = buf[:Put(buf, v)]
	}

	sort.Sort(randomCases)

	shouldBeLessThanOrEqual := func(actual interface{}, expected ...interface{}) string {
		a, b := actual.([]byte), expected[0].([]byte)
		if bytes.Compare(a, b) <= 0 {
			return fmt.Sprintf("Expected A <= B (but it wasn't)!\nA: [% x]\nB: [% x]", a, b)
		}
		return ""
	}

	Convey("TestSort", t, func() {
		prev := randomCases[0]
		for _, c := range randomCases[1:] {
			// Actually asserting with the So for every entry in the sorted array will
			// produce 100000 green checkmarks on a sucessful test, which is a bit
			// much :).
			if bytes.Compare(c.expect, prev.expect) < 0 {
				So(c.expect, shouldBeLessThanOrEqual, prev.expect)
				break
			}
			prev = c
		}

		// This silly assertion is done so that this test has a green check next to
		// it in the event that it passes. Otherwise convey thinks we skipped the
		// test, which isn't correct.
		So(true, ShouldBeTrue)
	})
}

type fakeWriter struct{ count int }

func (f *fakeWriter) WriteByte(byte) error {
	if f.count == 0 {
		return fmt.Errorf("nope")
	}
	f.count--
	return nil
}

func TestErrors(t *testing.T) {
	smallerInt64 := []byte{b01000000, b01111111, b11111111, b11111111, b11111111, 0xff, 0xff, 0xff, b11111110}

	prettyBigUint64 := []byte{b10111111, b10000000, 0, 0, 0, 0, 0, 0, 0}
	prettyBigUint64Val := uint64(math.MaxInt64 + 1)

	reallyBigUint64 := []byte{b10111111, b11111111, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
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
			buf:  []byte{b11000000}, // 65 bits!?
			n:    -1,
			un:   -1,
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

			un:   -2,
			uerr: ErrUnderflow,
		}, {
			name: "Reading a number smaller than min int64",
			buf:  smallerInt64,
			n:    -2,
			err:  ErrUnderflow,

			un:   -2,
			uerr: ErrUnderflow,
		}, {
			name: "Reading a number bigger than int64",
			buf:  prettyBigUint64,
			n:    -1,
			err:  ErrOverflow,

			uv: prettyBigUint64Val,
			un: len(prettyBigUint64),
		}, {
			name: "Reading MaxUint64",
			buf:  reallyBigUint64,
			n:    -1,
			err:  ErrOverflow,

			uv: reallyBigUint64Val,
			un: len(reallyBigUint64),
		},
	}

	Convey("Error conditions", t, func() {
		for _, t := range tests {
			Convey(t.name, func() {
				Convey("Get", func() {
					v, n := Get(t.buf)
					So(v, ShouldEqual, t.v)
					So(n, ShouldEqual, t.n)
				})
				Convey("GetUint", func() {
					uv, un := GetUint(t.buf)
					So(uv, ShouldEqual, t.uv)
					So(un, ShouldEqual, t.un)
				})
				Convey("Read", func() {
					v, err := Read(bytes.NewBuffer(t.buf))
					So(err, ShouldEqual, t.err)
					So(v, ShouldEqual, t.v)
				})
				Convey("ReadUint", func() {
					uv, err := ReadUint(bytes.NewBuffer(t.buf))
					So(err, ShouldEqual, t.uerr)
					So(uv, ShouldEqual, t.uv)
				})
			})
		}
		Convey("Panics", func() {
			Convey("Put", func() {
				buf := make([]byte, MaxFunnyBaseLen64)
				buf = buf[:4] // enough capacity, but not enough length!
				So(func() { Put(buf, cases[0].val) }, ShouldPanic)
			})
			Convey("PutUint", func() {
				buf := make([]byte, MaxFunnyBaseLen64)
				buf = buf[:4] // enough capacity, but not enough length!
				So(func() { PutUint(buf, reallyBigUint64Val) }, ShouldPanic)
			})
		})
		Convey("Write Errors", func() {
			// Test each error return location in writeSignMag
			for count := 0; count < 3; count++ {
				fw := &fakeWriter{count}
				err := Write(fw, -10000)
				So(err.Error(), ShouldContainSubstring, "nope")
				So(fw.count, ShouldEqual, 0)
			}
		})
	})
}
