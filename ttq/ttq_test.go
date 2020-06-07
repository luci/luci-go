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
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/ttq/datastore"
	"go.chromium.org/luci/ttq/internal"

	. "github.com/smartystreets/goconvey/convey"
)

func mkPartition(low, high int64) *internal.Partition {
	return internal.PartitionFromInts(low, high)
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

		t := New("", "", nil, datastore.New())

		Convey("Basic", func() {
			ls, err := t.loadLeasesInstrumented(ctx, shardId)
			So(err, ShouldBeNil)
			So(len(ls), ShouldEqual, 0)

			firstPartition := mkPartition(10, 14)
			can, err := t.canLease(ctx, firstPartition, shardId)
			So(err, ShouldBeNil)
			So(can, ShouldBeTrue)
		})

		Convey("Happy path", func() {
			firstPartition := mkPartition(10, 14)
			l, sp, err := t.doLease(ctx, firstPartition, shardId, clock.Now(ctx).Add(time.Minute))
			So(err, ShouldBeNil)
			So(l, ShouldNotBeNil)
			So(sp, ShouldResemble, internal.SortedPartitions{firstPartition})

			Convey("Concurrent lease", func() {
				clk.Add(59 * time.Second)

				Convey("Diff lease is OK", func() {
					another := mkPartition(20, 22)
					l2, sp2, err := t.doLease(ctx, another, shardId, clock.Now(ctx).Add(time.Minute))
					So(err, ShouldBeNil)
					So(l2, ShouldNotBeNil)
					So(sp2, ShouldResemble, internal.SortedPartitions{another})

					ls, err := t.loadLeasesInstrumented(ctx, shardId)
					So(err, ShouldBeNil)
					So(len(ls), ShouldEqual, 2)
				})

				Convey("Overlapping lease", func() {
					So(firstPartition, ShouldResemble, mkPartition(10, 14))
					overlapping := mkPartition(5, 20)

					l2, sp2, err := t.doLease(ctx, overlapping, shardId, clock.Now(ctx).Add(time.Minute))
					So(err, ShouldBeNil)
					So(l2, ShouldNotBeNil)
					So(sp2, ShouldResemble, internal.SortedPartitions{mkPartition(5, 10), mkPartition(14, 20)})

					evenBigger := mkPartition(1, 20)
					l3, sp3, err := t.doLease(ctx, evenBigger, shardId, clock.Now(ctx).Add(time.Minute))
					So(err, ShouldBeNil)
					So(l3, ShouldNotBeNil)
					So(sp3, ShouldResemble, internal.SortedPartitions{mkPartition(1, 5)})

					ls, err := t.loadLeasesInstrumented(ctx, shardId)
					So(err, ShouldBeNil)
					So(len(ls), ShouldEqual, 3)
				})

				Convey("Same lease is rejected", func() {
					_, _, err = t.doLease(ctx, firstPartition, shardId, clock.Now(ctx).Add(time.Minute))
					So(err, ShouldEqual, ErrInLease)

					ls, err := t.loadLeasesInstrumented(ctx, shardId)
					So(err, ShouldBeNil)
					So(ls, ShouldResemble, []*internal.Lease{l})
				})
			})

			Convey("Same lease but after prior expiry is allowed", func() {
				clk.Add(61 * time.Second)
				l2, sp2, err := t.doLease(ctx, firstPartition, shardId, clock.Now(ctx).Add(time.Minute))
				So(err, ShouldBeNil)
				So(l2, ShouldNotBeNil)
				So(sp2, ShouldResemble, internal.SortedPartitions{firstPartition})

				Convey("and expired lease should be cleaed up", func() {
					ls, err := t.loadLeasesInstrumented(ctx, shardId)
					So(err, ShouldBeNil)
					So(ls, ShouldResemble, []*internal.Lease{l2})
				})
			})
		})
	})
}
