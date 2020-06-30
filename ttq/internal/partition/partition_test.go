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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPartition(t *testing.T) {
	t.Parallel()

	Convey("Partition", t, func() {
		Convey("Universe", func() {
			u1 := Universe(1) // 1 byte
			So(u1.Low.Int64(), ShouldEqual, 0)
			So(u1.High.Int64(), ShouldEqual, 256)
			u4 := Universe(4) // 4 bytes
			So(u4.Low.Int64(), ShouldEqual, 0)
			So(u4.High.Int64(), ShouldEqual, int64(1)<<32)
		})

		Convey("To/From string", func() {
			u1 := Universe(1)
			So(u1.String(), ShouldEqual, "0_100")

			p, err := FromString("f0_ff")
			So(err, ShouldBeNil)
			So(p.Low.Int64(), ShouldEqual, 240)
			So(p.High.Int64(), ShouldEqual, 255)

			_, err = FromString("_")
			So(err, ShouldNotBeNil)
			_, err = FromString("10_")
			So(err, ShouldNotBeNil)
			_, err = FromString("_1")
			So(err, ShouldNotBeNil)
			_, err = FromString("10_-1")
			So(err, ShouldNotBeNil)
		})

		Convey("Span", func() {
			p, err := SpanInclusive("05", "10")
			So(err, ShouldBeNil)
			So(p.Low.Int64(), ShouldEqual, 5)
			So(p.High.Int64(), ShouldEqual, 0x10+1)

			_, err = SpanInclusive("Not hex", "10")
			So(err, ShouldNotBeNil)
		})

		Convey("Copy doesn't share bigInts", func() {
			var a, b *Partition
			a = FromInts(1, 10)
			b = a.Copy()
			a.Low.SetInt64(100)
			So(b, ShouldResemble, FromInts(1, 10))
		})

		Convey("ApplyToQuery", func() {
			u := Universe(1)
			l, h := u.QueryBounds(1)
			So(l, ShouldEqual, "00")
			So(h, ShouldEqual, "g")
			l, h = u.QueryBounds(2)
			So(l, ShouldEqual, "0000")
			So(h, ShouldEqual, "0100")
		})

		Convey("Split", func() {
			Convey("Exact", func() {
				u1 := Universe(1)
				ps := u1.Split(2)
				So(len(ps), ShouldEqual, 2)
				So(ps[0].Low.Int64(), ShouldEqual, 0)
				So(ps[0].High.Int64(), ShouldEqual, 128)
				So(ps[1].Low.Int64(), ShouldEqual, 128)
				So(ps[1].High.Int64(), ShouldEqual, 256)
			})

			Convey("Rounding", func() {
				ps := FromInts(0, 10).Split(3)
				So(len(ps), ShouldEqual, 3)
				So(ps[0].Low.Int64(), ShouldEqual, 0)
				So(ps[0].High.Int64(), ShouldEqual, 4)
				So(ps[1].Low.Int64(), ShouldEqual, 4)
				So(ps[1].High.Int64(), ShouldEqual, 8)
				So(ps[2].Low.Int64(), ShouldEqual, 8)
				So(ps[2].High.Int64(), ShouldEqual, 10)
			})

			Convey("Degenerate", func() {
				ps := FromInts(0, 1).Split(2)
				So(len(ps), ShouldEqual, 1)
				So(ps[0].Low.Int64(), ShouldEqual, 0)
				So(ps[0].High.Int64(), ShouldEqual, 1)
			})
		})

		Convey("EducatedSplitAfter", func() {
			u1 := Universe(1) // 0..256
			Convey("Ideal", func() {
				ps := u1.EducatedSplitAfter(
					"3f", // cutoff, covers 0..64
					8,    // items before the cutoff
					8,    // target per shard
					100,  // maxShards
				)
				So(len(ps), ShouldEqual, 3)
				So(ps[0].Low.Int64(), ShouldEqual, 64)
				So(ps[0].High.Int64(), ShouldEqual, 128)
				So(ps[1].Low.Int64(), ShouldEqual, 128)
				So(ps[1].High.Int64(), ShouldEqual, 192)
				So(ps[2].Low.Int64(), ShouldEqual, 192)
				So(ps[2].High.Int64(), ShouldEqual, 256)
			})
			Convey("MaxShards", func() {
				ps := u1.EducatedSplitAfter(
					"3f", // cutoff, covers 0..64
					8,    // items before the cutoff
					8,    // target per shard
					2,    // maxShards
				)
				So(len(ps), ShouldEqual, 2)
				So(ps[0].Low.Int64(), ShouldEqual, 64)
				So(ps[0].High.Int64(), ShouldEqual, 160)
				So(ps[1].Low.Int64(), ShouldEqual, 160)
				So(ps[1].High.Int64(), ShouldEqual, 256)
			})
			Convey("Rounding", func() {
				ps := u1.EducatedSplitAfter(
					"3f", // cutoff, covers 0..64
					8,    // items before the cutoff => (1/8 density)
					10,   // target per shard  => range of 80 per shard is ideal.
					100,  // maxShards
				)
				So(len(ps), ShouldEqual, 3)
				So(ps[0].Low.Int64(), ShouldEqual, 64)
				So(ps[0].High.Int64(), ShouldEqual, 128)
				So(ps[1].Low.Int64(), ShouldEqual, 128)
				So(ps[1].High.Int64(), ShouldEqual, 192)
				So(ps[2].Low.Int64(), ShouldEqual, 192)
				So(ps[2].High.Int64(), ShouldEqual, 256)
			})
		})
	})
}
