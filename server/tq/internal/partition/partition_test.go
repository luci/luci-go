// Copyright 2020 The LUCI Authors.
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

package partition

import (
	"encoding/json"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"math/big"
	"testing"
)

func TestPartition(t *testing.T) {
	t.Parallel()

	ftt.Run("Partition", t, func(t *ftt.Test) {
		t.Run("Universe", func(t *ftt.Test) {
			u1 := Universe(1) // 1 byte
			assert.Loosely(t, u1.Low.Int64(), should.BeZero)
			assert.Loosely(t, u1.High.Int64(), should.Equal(256))
			u4 := Universe(4) // 4 bytes
			assert.Loosely(t, u4.Low.Int64(), should.BeZero)
			assert.Loosely(t, u4.High.Int64(), should.Equal(int64(1)<<32))
		})

		t.Run("To/From string", func(t *ftt.Test) {
			u1 := Universe(1)
			assert.Loosely(t, u1.String(), should.Equal("0_100"))

			p, err := FromString("f0_ff")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, p.Low.Int64(), should.Equal(240))
			assert.Loosely(t, p.High.Int64(), should.Equal(255))

			_, err = FromString("_")
			assert.Loosely(t, err, should.NotBeNil)
			_, err = FromString("10_")
			assert.Loosely(t, err, should.NotBeNil)
			_, err = FromString("_1")
			assert.Loosely(t, err, should.NotBeNil)
			_, err = FromString("10_-1")
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("To/From JSON", func(t *ftt.Test) {
			t.Run("works", func(t *ftt.Test) {
				in := FromInts(5, 64)
				bytes, err := json.Marshal(in)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(bytes), should.Match(`"5_40"`))
				out := &Partition{}
				assert.Loosely(t, json.Unmarshal(bytes, out), should.BeNil)
				assert.Loosely(t, out, should.Resemble(in))
			})
			t.Run("null", func(t *ftt.Test) {
				p := Partition{}
				assert.Loosely(t, json.Unmarshal([]byte(`null`), &p), should.BeNil)
				assert.Loosely(t, p, should.Resemble(Partition{}))
			})
			t.Run("partial error doesn't mutate passed object", func(t *ftt.Test) {
				p := Partition{}
				err := json.Unmarshal([]byte(`"10_badhighvalue"`), &p)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, p, should.Resemble(Partition{}))
			})
		})

		t.Run("Span", func(t *ftt.Test) {
			p, err := SpanInclusive("05", "10")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, p.Low.Int64(), should.Equal(5))
			assert.Loosely(t, p.High.Int64(), should.Equal(0x10+1))

			_, err = SpanInclusive("Not hex", "10")
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Copy doesn't share bigInts", func(t *ftt.Test) {
			var a, b *Partition
			a = FromInts(1, 10)
			b = a.Copy()
			a.Low.SetInt64(100)
			assert.Loosely(t, b, should.Resemble(FromInts(1, 10)))
		})

		t.Run("ApplyToQuery", func(t *ftt.Test) {
			t.Run("inKeySpace", func(t *ftt.Test) {
				assert.Loosely(t, inKeySpace(big.NewInt(1), 1), should.BeTrue)
				assert.Loosely(t, inKeySpace(big.NewInt(254), 1), should.BeTrue)
				assert.Loosely(t, inKeySpace(big.NewInt(255), 1), should.BeTrue)
				assert.Loosely(t, inKeySpace(big.NewInt(256), 1), should.BeFalse)
				assert.Loosely(t, inKeySpace(big.NewInt(256), 2), should.BeTrue)
			})

			u := Universe(1)
			l, h := u.QueryBounds(1)
			assert.Loosely(t, l, should.Equal("00"))
			assert.Loosely(t, h, should.Equal("g"))
			l, h = u.QueryBounds(2)
			assert.Loosely(t, l, should.Equal("0000"))
			assert.Loosely(t, h, should.Equal("0100"))

			p := FromInts(10, 255)
			l, h = p.QueryBounds(1)
			assert.Loosely(t, l, should.Equal("0a"))
			assert.Loosely(t, h, should.Equal("ff"))
			l, h = p.QueryBounds(2)
			assert.Loosely(t, l, should.Equal("000a"))
			assert.Loosely(t, h, should.Equal("00ff"))
		})

		t.Run("Split", func(t *ftt.Test) {
			t.Run("Exact", func(t *ftt.Test) {
				u1 := Universe(1)
				ps := u1.Split(2)
				assert.Loosely(t, len(ps), should.Equal(2))
				assert.Loosely(t, ps[0].Low.Int64(), should.BeZero)
				assert.Loosely(t, ps[0].High.Int64(), should.Equal(128))
				assert.Loosely(t, ps[1].Low.Int64(), should.Equal(128))
				assert.Loosely(t, ps[1].High.Int64(), should.Equal(256))
			})

			t.Run("Rounding", func(t *ftt.Test) {
				ps := FromInts(0, 10).Split(3)
				assert.Loosely(t, len(ps), should.Equal(3))
				assert.Loosely(t, ps[0].Low.Int64(), should.BeZero)
				assert.Loosely(t, ps[0].High.Int64(), should.Equal(4))
				assert.Loosely(t, ps[1].Low.Int64(), should.Equal(4))
				assert.Loosely(t, ps[1].High.Int64(), should.Equal(8))
				assert.Loosely(t, ps[2].Low.Int64(), should.Equal(8))
				assert.Loosely(t, ps[2].High.Int64(), should.Equal(10))
			})

			t.Run("Degenerate", func(t *ftt.Test) {
				ps := FromInts(0, 1).Split(2)
				assert.Loosely(t, len(ps), should.Equal(1))
				assert.Loosely(t, ps[0].Low.Int64(), should.BeZero)
				assert.Loosely(t, ps[0].High.Int64(), should.Equal(1))
			})
		})

		t.Run("EducatedSplitAfter", func(t *ftt.Test) {
			u1 := Universe(1) // 0..256
			t.Run("Ideal", func(t *ftt.Test) {
				ps := u1.EducatedSplitAfter(
					"3f", // cutoff, covers 0..64
					8,    // items before the cutoff
					8,    // target per shard
					100,  // maxShards
				)
				assert.Loosely(t, len(ps), should.Equal(3))
				assert.Loosely(t, ps[0].Low.Int64(), should.Equal(64))
				assert.Loosely(t, ps[0].High.Int64(), should.Equal(128))
				assert.Loosely(t, ps[1].Low.Int64(), should.Equal(128))
				assert.Loosely(t, ps[1].High.Int64(), should.Equal(192))
				assert.Loosely(t, ps[2].Low.Int64(), should.Equal(192))
				assert.Loosely(t, ps[2].High.Int64(), should.Equal(256))
			})
			t.Run("MaxShards", func(t *ftt.Test) {
				ps := u1.EducatedSplitAfter(
					"3f", // cutoff, covers 0..64
					8,    // items before the cutoff
					8,    // target per shard
					2,    // maxShards
				)
				assert.Loosely(t, len(ps), should.Equal(2))
				assert.Loosely(t, ps[0].Low.Int64(), should.Equal(64))
				assert.Loosely(t, ps[0].High.Int64(), should.Equal(160))
				assert.Loosely(t, ps[1].Low.Int64(), should.Equal(160))
				assert.Loosely(t, ps[1].High.Int64(), should.Equal(256))
			})
			t.Run("Rounding", func(t *ftt.Test) {
				ps := u1.EducatedSplitAfter(
					"3f", // cutoff, covers 0..64
					8,    // items before the cutoff => (1/8 density)
					10,   // target per shard  => range of 80 per shard is ideal.
					100,  // maxShards
				)
				assert.Loosely(t, len(ps), should.Equal(3))
				assert.Loosely(t, ps[0].Low.Int64(), should.Equal(64))
				assert.Loosely(t, ps[0].High.Int64(), should.Equal(128))
				assert.Loosely(t, ps[1].Low.Int64(), should.Equal(128))
				assert.Loosely(t, ps[1].High.Int64(), should.Equal(192))
				assert.Loosely(t, ps[2].Low.Int64(), should.Equal(192))
				assert.Loosely(t, ps[2].High.Int64(), should.Equal(256))
			})
		})
	})
}

func TestSortedPartitionsBuilder(t *testing.T) {
	t.Parallel()

	ftt.Run("SortedPartitionsBuilder", t, func(t *ftt.Test) {
		b := NewSortedPartitionsBuilder(FromInts(0, 300))
		assert.Loosely(t, b.IsEmpty(), should.BeFalse)
		assert.Loosely(t, b.Result(), should.Resemble(SortedPartitions{FromInts(0, 300)}))

		b.Exclude(FromInts(100, 200))
		assert.Loosely(t, b.Result(), should.Resemble(SortedPartitions{FromInts(0, 100), FromInts(200, 300)}))

		b.Exclude(FromInts(150, 175)) // noop
		assert.Loosely(t, b.Result(), should.Resemble(SortedPartitions{FromInts(0, 100), FromInts(200, 300)}))

		b.Exclude(FromInts(150, 250)) // cut front
		assert.Loosely(t, b.Result(), should.Resemble(SortedPartitions{FromInts(0, 100), FromInts(250, 300)}))

		b.Exclude(FromInts(275, 400)) // cut back
		assert.Loosely(t, b.Result(), should.Resemble(SortedPartitions{FromInts(0, 100), FromInts(250, 275)}))

		b.Exclude(FromInts(40, 80)) // another split.
		assert.Loosely(t, b.Result(), should.Resemble(SortedPartitions{FromInts(0, 40), FromInts(80, 100), FromInts(250, 275)}))

		b.Exclude(FromInts(10, 270))
		assert.Loosely(t, b.Result(), should.Resemble(SortedPartitions{FromInts(0, 10), FromInts(270, 275)}))

		b.Exclude(FromInts(0, 4000))
		assert.Loosely(t, b.Result(), should.Resemble(SortedPartitions{}))
		assert.Loosely(t, b.IsEmpty(), should.BeTrue)
	})
}

func TestOnlyIn(t *testing.T) {
	t.Parallel()

	ftt.Run("SortedPartition.OnlyIn", t, func(t *ftt.Test) {
		var sp SortedPartitions

		const keySpaceBytes = 1
		copyIn := func(sorted ...string) []string {
			// This is actually the intended use of the OnlyIn function.
			reuse := sorted[:] // re-use existing sorted slice.
			l := 0
			key := func(i int) string {
				return sorted[i]
			}
			use := func(i, j int) {
				l += copy(reuse[l:], sorted[i:j])
			}
			sp.OnlyIn(len(sorted), key, use, keySpaceBytes)
			return reuse[:l]
		}

		sp = SortedPartitions{FromInts(1, 3), FromInts(9, 16), FromInts(241, 242)}
		assert.Loosely(t, copyIn("00"), should.Resemble([]string{}))
		assert.Loosely(t, copyIn("02"), should.Resemble([]string{"02"}))

		assert.Loosely(t, copyIn(
			"00",
			"02",
			"03",
			"0f", // 15
			"10", // 16
			"40", // 64
			"f0", // 240
			"f1", // 241
		),
			should.Resemble([]string{"02", "0f", "f1"}))
	})
}
