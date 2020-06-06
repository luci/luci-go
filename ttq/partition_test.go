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

package ttq

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	. "github.com/smartystreets/goconvey/convey"
	// . "go.chromium.org/luci/common/testing/assertions"
)

var epoch = time.Unix(1500000000, 0).UTC()

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
			l, h := u.queryBounds(1)
			So(l, ShouldEqual, "00")
			So(h, ShouldEqual, "g")
			l, h = u.queryBounds(2)
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
			So(sp.filter(nil, 1), ShouldBeNil)
			So(sp.filter([]*Reminder{&Reminder{ID: "00"}}, 1), ShouldBeNil)
			So(sp.filter([]*Reminder{&Reminder{ID: "02"}}, 1), ShouldResemble, []*Reminder{&Reminder{ID: "02"}})

			r := sp.filter([]*Reminder{
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
	p := &Partition{}
	p.low.SetInt64(low)
	p.high.SetInt64(high)
	return p
}

func TestLeases(t *testing.T) {
	t.Parallel()

	Convey("Leases", t, func() {
		ctx := memory.Use(context.Background())
		clk := testclock.New(testclock.TestRecentTimeLocal.Round(time.Minute))
		ctx = clock.Set(ctx, clk)
		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)
		shardId := 1

		Convey("Basic", func() {
			ls, err := loadLeases(ctx, shardId)
			So(err, ShouldBeNil)
			So(len(ls), ShouldEqual, 0)

			firstPartition := mkPartition(10, 14)
			can, err := canLease(ctx, firstPartition, shardId)
			So(err, ShouldBeNil)
			So(can, ShouldBeTrue)
		})

		Convey("Happy path", func() {
			firstPartition := mkPartition(10, 14)
			l, sp, err := doLease(ctx, firstPartition, shardId, clock.Now(ctx).Add(time.Minute))
			So(err, ShouldBeNil)
			So(l, ShouldNotBeNil)
			So(sp, ShouldResemble, SortedPartitions{firstPartition})

			Convey("Concurrent lease", func() {
				clk.Add(59 * time.Second)

				Convey("Diff lease is OK", func() {
					another := mkPartition(20, 22)
					l2, sp2, err := doLease(ctx, another, shardId, clock.Now(ctx).Add(time.Minute))
					So(err, ShouldBeNil)
					So(l2, ShouldNotBeNil)
					So(sp2, ShouldResemble, SortedPartitions{another})

					ls, err := loadLeases(ctx, shardId)
					So(err, ShouldBeNil)
					So(len(ls), ShouldEqual, 2)
				})

				Convey("Overlapping lease", func() {
					So(firstPartition, ShouldResemble, mkPartition(10, 14))
					overlapping := mkPartition(5, 20)

					l2, sp2, err := doLease(ctx, overlapping, shardId, clock.Now(ctx).Add(time.Minute))
					So(err, ShouldBeNil)
					So(l2, ShouldNotBeNil)
					So(sp2, ShouldResemble, SortedPartitions{mkPartition(5, 10), mkPartition(14, 20)})

					evenBigger := mkPartition(1, 20)
					l3, sp3, err := doLease(ctx, evenBigger, shardId, clock.Now(ctx).Add(time.Minute))
					So(err, ShouldBeNil)
					So(l3, ShouldNotBeNil)
					So(sp3, ShouldResemble, SortedPartitions{mkPartition(1, 5)})

					ls, err := loadLeases(ctx, shardId)
					So(err, ShouldBeNil)
					So(len(ls), ShouldEqual, 3)
				})

				Convey("Same lease is rejected", func() {
					_, _, err = doLease(ctx, firstPartition, shardId, time.Time{}) // expiry for concurrent lease doesn't matter.
					So(err, ShouldEqual, ErrInLease)

					ls, err := loadLeases(ctx, shardId)
					So(err, ShouldBeNil)
					So(ls, ShouldResemble, []*Lease{l})
				})
			})

			Convey("Same lease but after prior expiry is allowed", func() {
				clk.Add(61 * time.Second)
				l2, sp2, err := doLease(ctx, firstPartition, shardId, clock.Now(ctx).Add(time.Minute))
				So(err, ShouldBeNil)
				So(l2, ShouldNotBeNil)
				So(sp2, ShouldResemble, SortedPartitions{firstPartition})

				Convey("and expired lease should be cleaed up", func() {
					ls, err := loadLeases(ctx, shardId)
					So(err, ShouldBeNil)
					So(ls, ShouldResemble, []*Lease{l2})
				})
			})
		})
	})
}
