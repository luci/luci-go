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

package internal

import (
	"context"
	"testing"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPartition(t *testing.T) {
	t.Parallel()

	Convey("Partition", t, func() {
		Convey("Universe", func() {
			u1 := Universe(1) // 1 byte
			So(u1.low.Int64(), ShouldEqual, 0)
			So(u1.high.Int64(), ShouldEqual, 256)
			u4 := Universe(4) // 4 bytes
			So(u4.low.Int64(), ShouldEqual, 0)
			So(u4.high.Int64(), ShouldEqual, int64(1)<<32)
		})

		Convey("To/From string", func() {
			u1 := Universe(1)
			So(u1.String(), ShouldEqual, "0_100")

			p, err := PartitionFromString("f0_ff")
			So(err, ShouldBeNil)
			So(p.low.Int64(), ShouldEqual, 240)
			So(p.high.Int64(), ShouldEqual, 255)
		})

		Convey("Split power of 2", func() {
			u1 := Universe(1)
			ps := u1.Split(2)
			So(len(ps), ShouldEqual, 2)
			So(ps[0].low.Int64(), ShouldEqual, 0)
			So(ps[0].high.Int64(), ShouldEqual, 128)
			So(ps[1].low.Int64(), ShouldEqual, 128)
			So(ps[1].high.Int64(), ShouldEqual, 256)
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
				So(ps[0].low.Int64(), ShouldEqual, 64)
				So(ps[0].high.Int64(), ShouldEqual, 128)
				So(ps[1].low.Int64(), ShouldEqual, 128)
				So(ps[1].high.Int64(), ShouldEqual, 192)
				So(ps[2].low.Int64(), ShouldEqual, 192)
				So(ps[2].high.Int64(), ShouldEqual, 256)
			})
			Convey("MaxShards", func() {
				ps := u1.EducatedSplitAfter(
					"3f", // cutoff, covers 0..64
					8,    // items before the cutoff
					8,    // target per shard
					2,    // maxShards
				)
				So(len(ps), ShouldEqual, 2)
				So(ps[0].low.Int64(), ShouldEqual, 64)
				So(ps[0].high.Int64(), ShouldEqual, 160)
				So(ps[1].low.Int64(), ShouldEqual, 160)
				So(ps[1].high.Int64(), ShouldEqual, 256)
			})
			Convey("Rounding", func() {
				ps := u1.EducatedSplitAfter(
					"3f", // cutoff, covers 0..64
					8,    // items before the cutoff => (1/8 density)
					10,   // target per shard  => range of 80 per shard is ideal.
					100,  // maxShards
				)
				So(len(ps), ShouldEqual, 3)
				So(ps[0].low.Int64(), ShouldEqual, 64)
				So(ps[0].high.Int64(), ShouldEqual, 128)
				So(ps[1].low.Int64(), ShouldEqual, 128)
				So(ps[1].high.Int64(), ShouldEqual, 192)
				So(ps[2].low.Int64(), ShouldEqual, 192)
				So(ps[2].high.Int64(), ShouldEqual, 256)
			})
		})

		Convey("SortedPartitionsBuilder", func() {
			b := NewSortedPartitionsBuilder(mkPartition(0, 300))
			So(b.IsEmpty(), ShouldBeFalse)
			So(b.Result(), ShouldResemble, SortedPartitions{mkPartition(0, 300)})

			b.Exclude(mkPartition(100, 200))
			So(b.Result(), ShouldResemble, SortedPartitions{mkPartition(0, 100), mkPartition(200, 300)})

			b.Exclude(mkPartition(150, 175)) // noop
			So(b.Result(), ShouldResemble, SortedPartitions{mkPartition(0, 100), mkPartition(200, 300)})

			b.Exclude(mkPartition(150, 250)) // cut front
			So(b.Result(), ShouldResemble, SortedPartitions{mkPartition(0, 100), mkPartition(250, 300)})

			b.Exclude(mkPartition(275, 400)) // cut back
			So(b.Result(), ShouldResemble, SortedPartitions{mkPartition(0, 100), mkPartition(250, 275)})

			b.Exclude(mkPartition(40, 80)) // another split.
			So(b.Result(), ShouldResemble, SortedPartitions{mkPartition(0, 40), mkPartition(80, 100), mkPartition(250, 275)})

			b.Exclude(mkPartition(10, 270))
			So(b.Result(), ShouldResemble, SortedPartitions{mkPartition(0, 10), mkPartition(270, 275)})

			b.Exclude(mkPartition(0, 4000))
			So(b.Result(), ShouldResemble, SortedPartitions{})
			So(b.IsEmpty(), ShouldBeTrue)
		})

		Convey("Span", func() {
			p := PartitionSpanning([]*Reminder{
				&Reminder{ID: "05"},
				&Reminder{ID: "0f"},
				&Reminder{ID: "10"},
			})
			So(p.low.Int64(), ShouldEqual, 5)
			So(p.high.Int64(), ShouldEqual, 17) // 0x10 + 1.
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

		Convey("Temp", func() {
			ctx := memory.Use(context.Background())
			So(ds.Put(ctx, &Reminder{ID: "0100"}), ShouldBeNil)
			So(ds.Put(ctx, &Reminder{ID: "0200"}), ShouldBeNil)
			So(ds.Put(ctx, &Reminder{ID: "0300"}), ShouldBeNil)
			ds.GetTestable(ctx).CatchupIndexes()

			var r []*Reminder
			So(ds.GetAll(ctx, ds.NewQuery("ttq.Task").KeysOnly(true), &r), ShouldBeNil)
			So(len(r), ShouldEqual, 3)

			q := ds.NewQuery("ttq.Task").Order("__key__")
			q = q.Gt("__key__", ds.KeyForObj(ctx, &Reminder{ID: "0100"})).KeysOnly(true)
			r = nil
			So(ds.GetAll(ctx, q, &r), ShouldBeNil)
			So(len(r), ShouldEqual, 2)
		})

		Convey("Filter", func() {
			sp := SortedPartitions{mkPartition(1, 3), mkPartition(9, 16), mkPartition(241, 242)}
			So(sp.Filter(nil, 1), ShouldBeNil)
			So(sp.Filter([]*Reminder{&Reminder{ID: "00"}}, 1), ShouldBeNil)
			So(sp.Filter([]*Reminder{&Reminder{ID: "02"}}, 1), ShouldResemble, []*Reminder{&Reminder{ID: "02"}})

			r := sp.Filter([]*Reminder{
				&Reminder{ID: "00"},
				&Reminder{ID: "02"},
				&Reminder{ID: "03"},
				&Reminder{ID: "0f"}, // 15
				&Reminder{ID: "10"}, // 16
				&Reminder{ID: "40"}, // 64
				&Reminder{ID: "f0"}, // 240
				&Reminder{ID: "f1"}, // 241
			}, 1) // 1-byte keyspace.
			So(r, ShouldResemble, []*Reminder{
				&Reminder{ID: "02"},
				&Reminder{ID: "0f"}, // 15
				&Reminder{ID: "f1"}, // 241
			})

		})
	})
}

func mkPartition(low, high int64) *Partition {
	return PartitionFromInts(low, high)
}
