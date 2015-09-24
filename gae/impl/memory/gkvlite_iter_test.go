// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"testing"

	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/gkvlite"
	"github.com/luci/luci-go/common/cmpbin"
	. "github.com/smartystreets/goconvey/convey"
)

func mkNum(n int64) []byte {
	buf := &bytes.Buffer{}
	_, err := cmpbin.WriteInt(buf, n)
	memoryCorruption(err)

	return buf.Bytes()
}

func readNum(data []byte) int64 {
	ret, _, err := cmpbin.ReadInt(bytes.NewBuffer(data))
	memoryCorruption(err)

	return ret
}

func TestIterator(t *testing.T) {
	t.Parallel()

	s := newMemStore()
	c := s.SetCollection("zup", nil)
	prev := []byte{}
	for i := 5; i < 100; i++ {
		data := mkNum(int64(i))
		c.Set(data, prev)
		prev = data
	}

	get := func(c C, t *iterator) interface{} {
		ret := interface{}(nil)
		t.next(nil, func(i *gkvlite.Item) {
			if i != nil {
				ret = readNum(i.Key)
			}
		})
		return ret
	}

	skipGet := func(c C, t *iterator, skipTo int64) interface{} {
		ret := interface{}(nil)
		t.next(mkNum(skipTo), func(i *gkvlite.Item) {
			if i != nil {
				ret = readNum(i.Key)
			}
		})
		return ret
	}

	Convey("Test iterator", t, func() {
		Convey("start at nil", func(ctx C) {
			t := (&iterDefinition{c: c}).mkIter()
			defer t.stop()
			So(get(ctx, t), ShouldEqual, 5)
			So(get(ctx, t), ShouldEqual, 6)
			So(get(ctx, t), ShouldEqual, 7)

			Convey("And can skip", func(ctx C) {
				So(skipGet(ctx, t, 10), ShouldEqual, 10)
				So(get(ctx, t), ShouldEqual, 11)

				Convey("But not forever", func(ctx C) {
					t.next(mkNum(200), func(i *gkvlite.Item) {
						ctx.So(i, ShouldBeNil)
					})
					t.next(nil, func(i *gkvlite.Item) {
						ctx.So(i, ShouldBeNil)
					})
				})
			})

			Convey("Can iterate explicitly", func(ctx C) {
				So(skipGet(ctx, t, 7), ShouldEqual, 8)
				So(skipGet(ctx, t, 8), ShouldEqual, 9)

				// Giving the immediately next key doesn't cause an internal reset.
				So(skipGet(ctx, t, 10), ShouldEqual, 10)
			})

			Convey("Can stop", func(ctx C) {
				t.stop()
				t.next(mkNum(200), func(i *gkvlite.Item) {
					ctx.So(i, ShouldBeNil)
				})
				t.next(nil, func(i *gkvlite.Item) {
					ctx.So(i, ShouldBeNil)
				})
				So(t.stop, ShouldNotPanic)
			})

			Convey("Going backwards is ignored", func(ctx C) {
				So(skipGet(ctx, t, 3), ShouldEqual, 8)
				So(get(ctx, t), ShouldEqual, 9)
				So(skipGet(ctx, t, 20), ShouldEqual, 20)
				So(get(ctx, t), ShouldEqual, 21)
			})

			Convey("will stop at the end of the list", func(ctx C) {
				So(skipGet(ctx, t, 95), ShouldEqual, 95)
				So(get(ctx, t), ShouldEqual, 96)
				So(get(ctx, t), ShouldEqual, 97)
				So(get(ctx, t), ShouldEqual, 98)
				So(get(ctx, t), ShouldEqual, 99)
				So(get(ctx, t), ShouldBeNil)
				So(get(ctx, t), ShouldBeNil)
			})
		})

		Convey("can have caps on both sides", func(ctx C) {
			t := (&iterDefinition{c: c, start: mkNum(20), end: mkNum(25)}).mkIter()
			So(get(ctx, t), ShouldEqual, 20)
			So(get(ctx, t), ShouldEqual, 21)
			So(get(ctx, t), ShouldEqual, 22)
			So(get(ctx, t), ShouldEqual, 23)
			So(get(ctx, t), ShouldEqual, 24)
			t.next(nil, func(i *gkvlite.Item) {
				ctx.So(i, ShouldBeNil)
			})
		})

		Convey("can skip over starting cap", func(ctx C) {
			t := (&iterDefinition{c: c, start: mkNum(20), end: mkNum(25)}).mkIter()
			So(skipGet(ctx, t, 22), ShouldEqual, 22)
			So(get(ctx, t), ShouldEqual, 23)
			So(get(ctx, t), ShouldEqual, 24)
			t.next(nil, func(i *gkvlite.Item) {
				ctx.So(i, ShouldBeNil)
			})
		})

	})
}

func TestMultiIteratorSimple(t *testing.T) {
	t.Parallel()

	// Simulate an index with 2 columns (int and int).
	vals := [][]int64{
		{1, 0},
		{1, 2},
		{1, 4},
		{1, 7},
		{1, 9},
		{3, 10},
		{3, 11},
	}

	valBytes := make([][]byte, len(vals))
	for i, nms := range vals {
		numbs := make([][]byte, len(nms))
		for j, n := range nms {
			numbs[j] = mkNum(n)
		}
		valBytes[i] = serialize.Join(numbs...)
	}

	otherVals := [][]int64{
		{3, 0},
		{4, 10},
		{19, 7},
		{20, 2},
		{20, 3},
		{20, 4},
		{20, 8},
		{20, 11},
	}

	otherValBytes := make([][]byte, len(otherVals))
	for i, nms := range otherVals {
		numbs := make([][]byte, len(nms))
		for i, n := range nms {
			numbs[i] = mkNum(n)
		}
		otherValBytes[i] = serialize.Join(numbs...)
	}

	Convey("Test MultiIterator", t, func() {
		s := newMemStore()
		c := s.SetCollection("zup1", nil)
		for _, row := range valBytes {
			c.Set(row, []byte{})
		}
		c2 := s.SetCollection("zup2", nil)
		for _, row := range otherValBytes {
			c2.Set(row, []byte{})
		}

		Convey("can join the same collection twice", func() {
			// get just the (1, *)
			// starting at (1, 2) (i.e. >= 2)
			// ending at (1, 4) (i.e. < 7)
			defs := []*iterDefinition{
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1)), start: mkNum(2), end: mkNum(7)},
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1)), start: mkNum(2), end: mkNum(7)},
			}

			i := 1
			multiIterate(defs, func(suffix []byte) bool {
				So(readNum(suffix), ShouldEqual, vals[i][1])
				i++
				return true
			})

			So(i, ShouldEqual, 3)
		})

		Convey("can make empty iteration", func() {
			// get just the (20, *) (doesn't exist)
			defs := []*iterDefinition{
				{c: c, prefix: mkNum(20)},
				{c: c, prefix: mkNum(20)},
			}

			i := 0
			multiIterate(defs, func(suffix []byte) bool {
				panic("never")
			})

			So(i, ShouldEqual, 0)
		})

		Convey("can join (other, val, val)", func() {
			// 'other' must start with 20, 'vals' must start with 1
			// no range constraints
			defs := []*iterDefinition{
				{c: c2, prefix: mkNum(20)},
				{c: c, prefix: mkNum(1)},
				{c: c, prefix: mkNum(1)},
			}

			expect := []int64{2, 4}
			i := 0
			multiIterate(defs, func(suffix []byte) bool {
				So(readNum(suffix), ShouldEqual, expect[i])
				i++
				return true
			})
		})

		Convey("Can stop early", func() {
			defs := []*iterDefinition{
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1))},
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1))},
			}

			i := 0
			multiIterate(defs, func(suffix []byte) bool {
				So(readNum(suffix), ShouldEqual, vals[i][1])
				i++
				return true
			})
			So(i, ShouldEqual, 5)

			i = 0
			multiIterate(defs, func(suffix []byte) bool {
				So(readNum(suffix), ShouldEqual, vals[i][1])
				i++
				return false
			})
			So(i, ShouldEqual, 1)
		})

	})

}
