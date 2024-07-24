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

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/service/datastore"
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

func TestIterator(iter *testing.T) {
	iter.Parallel()

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

	get := func(iter *iterator) any {
		if ent := iter.next(); ent != nil {
			return readNum(ent.key)
		}
		return nil
	}

	skipGet := func(iter *iterator, skipTo int64) any {
		iter.skip(mkNum(skipTo))
		return get(iter)
	}

	didIterate := func(iter *iterator) (did bool) {
		iterCB(iter, func(k, v []byte) {
			did = true
		})
		return
	}

	ftt.Run("Test iterator", iter, func(t *ftt.Test) {
		t.Run("start at nil", func(t *ftt.Test) {
			iter := (&iterDefinition{c: c}).mkIter()
			assert.Loosely(t, get(iter), should.Equal(5))
			assert.Loosely(t, get(iter), should.Equal(6))
			assert.Loosely(t, get(iter), should.Equal(7))

			t.Run("And can skip", func(t *ftt.Test) {
				assert.Loosely(t, skipGet(iter, 10), should.Equal(10))
				assert.Loosely(t, get(iter), should.Equal(11))

				t.Run("But not forever", func(t *ftt.Test) {
					iter.skip(mkNum(200))
					assert.Loosely(t, didIterate(iter), should.BeFalse)
					assert.Loosely(t, didIterate(iter), should.BeFalse)
				})
			})

			t.Run("Can iterate explicitly", func(t *ftt.Test) {
				assert.Loosely(t, skipGet(iter, 7), should.Equal(8))
				assert.Loosely(t, skipGet(iter, 8), should.Equal(9))

				// Giving the immediately next key doesn't cause an internal reset.
				assert.Loosely(t, skipGet(iter, 10), should.Equal(10))
			})

			t.Run("Going backwards is ignored", func(t *ftt.Test) {
				assert.Loosely(t, skipGet(iter, 3), should.Equal(8))
				assert.Loosely(t, get(iter), should.Equal(9))
				assert.Loosely(t, skipGet(iter, 20), should.Equal(20))
				assert.Loosely(t, get(iter), should.Equal(21))
			})

			t.Run("will stop at the end of the list", func(t *ftt.Test) {
				assert.Loosely(t, skipGet(iter, 95), should.Equal(95))
				assert.Loosely(t, get(iter), should.Equal(96))
				assert.Loosely(t, get(iter), should.Equal(97))
				assert.Loosely(t, get(iter), should.Equal(98))
				assert.Loosely(t, get(iter), should.Equal(99))
				assert.Loosely(t, get(iter), should.BeNil)
				assert.Loosely(t, get(iter), should.BeNil)
			})
		})

		t.Run("can have caps on both sides", func(t *ftt.Test) {
			iter := (&iterDefinition{c: c, start: mkNum(20), end: mkNum(25)}).mkIter()
			assert.Loosely(t, get(iter), should.Equal(20))
			assert.Loosely(t, get(iter), should.Equal(21))
			assert.Loosely(t, get(iter), should.Equal(22))
			assert.Loosely(t, get(iter), should.Equal(23))
			assert.Loosely(t, get(iter), should.Equal(24))

			assert.Loosely(t, didIterate(iter), should.BeFalse)
		})

		t.Run("can skip over starting cap", func(t *ftt.Test) {
			iter := (&iterDefinition{c: c, start: mkNum(20), end: mkNum(25)}).mkIter()
			assert.Loosely(t, skipGet(iter, 22), should.Equal(22))
			assert.Loosely(t, get(iter), should.Equal(23))
			assert.Loosely(t, get(iter), should.Equal(24))

			assert.Loosely(t, didIterate(iter), should.BeFalse)
		})
	})
}

func TestMultiIteratorSimple(iter *testing.T) {
	iter.Parallel()

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
		valBytes[i] = cmpbin.ConcatBytes(numbs...)
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
		otherValBytes[i] = cmpbin.ConcatBytes(numbs...)
	}

	ftt.Run("Test MultiIterator", iter, func(t *ftt.Test) {
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

		t.Run("can join the same collection twice", func(t *ftt.Test) {
			// get just the (1, *)
			// starting at (1, 2) (i.e. >= 2)
			// ending at (1, 4) (i.e. < 7)
			defs := []*iterDefinition{
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1)), start: mkNum(2), end: mkNum(7)},
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1)), start: mkNum(2), end: mkNum(7)},
			}

			i := 1
			assert.Loosely(t, multiIterate(defs, func(suffix []byte) error {
				assert.Loosely(t, readNum(suffix), should.Equal(vals[i][1]))
				i++
				return nil
			}), shouldBeSuccessful)

			assert.Loosely(t, i, should.Equal(3))
		})

		t.Run("can make empty iteration", func(t *ftt.Test) {
			// get just the (20, *) (doesn't exist)
			defs := []*iterDefinition{
				{c: c, prefix: mkNum(20)},
				{c: c, prefix: mkNum(20)},
			}

			i := 0
			assert.Loosely(t, multiIterate(defs, func(suffix []byte) error {
				panic("never")
			}), shouldBeSuccessful)

			assert.Loosely(t, i, should.BeZero)
		})

		t.Run("can join (other, val, val)", func(t *ftt.Test) {
			// 'other' must start with 20, 'vals' must start with 1
			// no range constraints
			defs := []*iterDefinition{
				{c: c2, prefix: mkNum(20)},
				{c: c, prefix: mkNum(1)},
				{c: c, prefix: mkNum(1)},
			}

			expect := []int64{2, 4}
			i := 0
			assert.Loosely(t, multiIterate(defs, func(suffix []byte) error {
				assert.Loosely(t, readNum(suffix), should.Equal(expect[i]))
				i++
				return nil
			}), shouldBeSuccessful)
		})

		t.Run("Can stop early", func(t *ftt.Test) {
			defs := []*iterDefinition{
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1))},
				{c: c, prefix: mkNum(1), prefixLen: len(mkNum(1))},
			}

			i := 0
			assert.Loosely(t, multiIterate(defs, func(suffix []byte) error {
				assert.Loosely(t, readNum(suffix), should.Equal(vals[i][1]))
				i++
				return nil
			}), shouldBeSuccessful)
			assert.Loosely(t, i, should.Equal(5))

			i = 0
			assert.Loosely(t, multiIterate(defs, func(suffix []byte) error {
				assert.Loosely(t, readNum(suffix), should.Equal(vals[i][1]))
				i++
				return datastore.Stop
			}), shouldBeSuccessful)
			assert.Loosely(t, i, should.Equal(1))
		})

	})

}
