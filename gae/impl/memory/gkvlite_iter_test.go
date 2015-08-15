// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/luci/gkvlite"
	"github.com/luci/luci-go/common/cmpbin"
	. "github.com/smartystreets/goconvey/convey"
)

func TestIterator(t *testing.T) {
	t.Parallel()

	mkNum := func(n int64) []byte {
		buf := &bytes.Buffer{}
		_, err := cmpbin.WriteInt(buf, n)
		if err != nil {
			panic(fmt.Errorf("your RAM is busted: %s", err))
		}
		return buf.Bytes()
	}

	getNum := func(data []byte) int64 {
		ret, _, err := cmpbin.ReadInt(bytes.NewBuffer(data))
		if err != nil {
			panic(fmt.Errorf("your RAM is (probably) busted: %s", err))
		}
		return ret
	}

	s := newMemStore()
	c := s.SetCollection("zup", nil)
	prev := []byte{}
	for i := 5; i < 100; i++ {
		data := mkNum(int64(i))
		c.Set(data, prev)
		prev = data
	}

	get := func(c C, t *iterator) int64 {
		ret := int64(0)
		t.next(nil, func(i *gkvlite.Item) {
			c.So(i, ShouldNotBeNil)
			ret = getNum(i.Key)
		})
		return ret
	}

	skipGet := func(c C, t *iterator, skipTo int64) int64 {
		ret := int64(0)
		t.next(mkNum(skipTo), func(i *gkvlite.Item) {
			c.So(i, ShouldNotBeNil)
			ret = getNum(i.Key)
		})
		return ret
	}

	Convey("Test iterator", t, func() {
		Convey("start at nil", func(ctx C) {
			t := newIterable(c, nil)
			So(get(ctx, t), ShouldEqual, 5)
			So(get(ctx, t), ShouldEqual, 6)
			So(get(ctx, t), ShouldEqual, 7)

			Convey("And can skip", func() {
				So(skipGet(ctx, t, 10), ShouldEqual, 10)
				So(get(ctx, t), ShouldEqual, 11)

				Convey("But not forever", func(c C) {
					t.next(mkNum(200), func(i *gkvlite.Item) {
						c.So(i, ShouldBeNil)
					})
					t.next(nil, func(i *gkvlite.Item) {
						c.So(i, ShouldBeNil)
					})
				})
			})

			Convey("Can stop", func(c C) {
				t.stop()
				t.next(mkNum(200), func(i *gkvlite.Item) {
					c.So(i, ShouldBeNil)
				})
				t.next(nil, func(i *gkvlite.Item) {
					c.So(i, ShouldBeNil)
				})
				So(t.stop, ShouldNotPanic)
			})

			Convey("Going backwards is fine", func(c C) {
				So(skipGet(ctx, t, 3), ShouldEqual, 5)
				So(get(ctx, t), ShouldEqual, 6)
			})
		})

		Convey("can have caps on both sides", func(ctx C) {
			t := newIterable(c, mkNum(25))
			So(skipGet(ctx, t, 20), ShouldEqual, 20)
			So(get(ctx, t), ShouldEqual, 21)
			So(get(ctx, t), ShouldEqual, 22)
			So(get(ctx, t), ShouldEqual, 23)
			So(get(ctx, t), ShouldEqual, 24)
			t.next(nil, func(i *gkvlite.Item) {
				ctx.So(i, ShouldBeNil)
			})
		})
	})
}
