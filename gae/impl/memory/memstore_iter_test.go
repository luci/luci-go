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

package memory

import (
	"bytes"
	"testing"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"

	"go.chromium.org/luci/common/data/cmpbin"

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

func countItems(mc memCollection) int {
	count := 0
	mc.ForEachItem(func(_, _ []byte) bool {
		count++
		return true
	})
	return count
}

func TestIterator(t *testing.T) {
	t.Parallel()

	s := newMemStore()
	c := s.GetOrCreateCollection("zup")
	prev := []byte{}
	for i := 5; i < 100; i++ {
		data := mkNum(int64(i))
		c.Set(data, prev)
		prev = data
	}
	c = s.Snapshot().GetCollection("zup")

	iterCB := func(it *iterator, cb func(k, v []byte)) {
		for ent := it.next(); ent != nil; ent = it.next() {
			cb(ent.key, ent.value)
		}
	}

	get := func(c C, t *iterator) interface{} {
		if ent := t.next(); ent != nil {
			return readNum(ent.key)
		}
		return nil
	}

	skipGet := func(c C, t *iterator, skipTo int64) interface{} {
		t.skip(mkNum(skipTo))
		return get(c, t)
	}

	didIterate := func(t *iterator) (did bool) {
		iterCB(t, func(k, v []byte) {
			did = true
		})
		return
	}

	Convey("Test iterator", t, func() {
		Convey("start at nil", func(ctx C) {
			t := (&iterDefinition{c: c}).mkIter()
			So(get(ctx, t), ShouldEqual, 5)
			So(get(ctx, t), ShouldEqual, 6)
			So(get(ctx, t), ShouldEqual, 7)

			Convey("And can skip", func(ctx C) {
				So(skipGet(ctx, t, 10), ShouldEqual, 10)
				So(get(ctx, t), ShouldEqual, 11)

				Convey("But not forever", func(ctx C) {
					t.skip(mkNum(200))
					So(didIterate(t), ShouldBeFalse)
					So(didIterate(t), ShouldBeFalse)
				})
			})

			Convey("Can iterate explicitly", func(ctx C) {
				So(skipGet(ctx, t, 7), ShouldEqual, 8)
				So(skipGet(ctx, t, 8), ShouldEqual, 9)

				// Giving the immediately next key doesn't cause an internal reset.
				So(skipGet(ctx, t, 10), ShouldEqual, 10)
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

			So(didIterate(t), ShouldBeFalse)
		})

		Convey("can skip over starting cap", func(ctx C) {
			t := (&iterDefinition{c: c, start: mkNum(20), end: mkNum(25)}).mkIter()
			So(skipGet(ctx, t, 22), ShouldEqual, 22)
			So(get(ctx, t), ShouldEqual, 23)
			So(get(ctx, t), ShouldEqual, 24)

			So(didIterate(t), ShouldBeFalse)
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
		c := s.GetOrCreateCollection("zup1")
		for _, row := range valBytes {
			c.Set(row, []byte{})
		}
		c2 := s.GetOrCreateCollection("zup2")
		for _, row := range otherValBytes {
			c2.Set(row, []byte{})
		}
		c = s.Snapshot().GetCollection("zup1")
		c2 = s.Snapshot().GetCollection("zup2")

		Convey("can join the same collection twice", func() {
			// get just the (1, *)
			// starting at (1, 2) (i.e. >= 2)
			// ending at (1, 4) (i.e. < 7)
			defs := []*iterDefinition{
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1)), start: mkNum(2), end: mkNum(7)},
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1)), start: mkNum(2), end: mkNum(7)},
			}

			i := 1
			So(multiIterate(defs, func(suffix []byte) error {
				So(readNum(suffix), ShouldEqual, vals[i][1])
				i++
				return nil
			}), shouldBeSuccessful)

			So(i, ShouldEqual, 3)
		})

		Convey("can make empty iteration", func() {
			// get just the (20, *) (doesn't exist)
			defs := []*iterDefinition{
				{c: c, prefix: mkNum(20)},
				{c: c, prefix: mkNum(20)},
			}

			i := 0
			So(multiIterate(defs, func(suffix []byte) error {
				panic("never")
			}), shouldBeSuccessful)

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
			So(multiIterate(defs, func(suffix []byte) error {
				So(readNum(suffix), ShouldEqual, expect[i])
				i++
				return nil
			}), shouldBeSuccessful)
		})

		Convey("Can stop early", func() {
			defs := []*iterDefinition{
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1))},
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1))},
			}

			i := 0
			So(multiIterate(defs, func(suffix []byte) error {
				So(readNum(suffix), ShouldEqual, vals[i][1])
				i++
				return nil
			}), shouldBeSuccessful)
			So(i, ShouldEqual, 5)

			i = 0
			So(multiIterate(defs, func(suffix []byte) error {
				So(readNum(suffix), ShouldEqual, vals[i][1])
				i++
				return datastore.Stop
			}), shouldBeSuccessful)
			So(i, ShouldEqual, 1)
		})

	})

}
