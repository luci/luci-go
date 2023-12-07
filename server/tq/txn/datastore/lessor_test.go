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

package datastore

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/tq/internal/partition"

	. "github.com/smartystreets/goconvey/convey"
)

const (
	sectionA = "sectionA"
	sectionB = "sectionB"
)

func TestLeasing(t *testing.T) {
	t.Parallel()

	epoch := ds.RoundTime(testclock.TestRecentTimeLocal)

	Convey("leasing works", t, func() {
		ctx := memory.Use(context.Background())
		ctx, tclock := testclock.UseTime(ctx, epoch)
		lessor := dsLessor{}

		Convey("noop for save and load", func() {
			l, err := save(ctx, sectionA, epoch.Add(time.Minute), nil)
			So(err, ShouldBeNil)
			So(l.Id, ShouldEqual, 0)

			active, expired, err := loadAll(ctx, sectionA)
			So(err, ShouldBeNil)
			So(len(active)+len(expired), ShouldEqual, 0)
		})

		// Save 3 leases with 1, 2, 3 minutes expiry, respectively.
		l1, err := save(ctx, sectionA, epoch.Add(time.Minute), partition.SortedPartitions{
			partition.FromInts(10, 15),
		})
		So(err, ShouldBeNil)
		l2, err := save(ctx, sectionA, epoch.Add(2*time.Minute), partition.SortedPartitions{
			partition.FromInts(20, 25),
		})
		So(err, ShouldBeNil)
		l3, err := save(ctx, sectionA, epoch.Add(3*time.Minute), partition.SortedPartitions{
			partition.FromInts(30, 35),
		})
		So(err, ShouldBeNil)
		l1.parts = nil
		l2.parts = nil
		l3.parts = nil

		Convey("diff shard", func() {
			active, expired, err := loadAll(ctx, sectionB)
			So(err, ShouldBeNil)
			So(len(active)+len(expired), ShouldEqual, 0)

			Convey("WithLease sets context deadline at lease expiry", func() {
				i := inLease{}
				err = lessor.WithLease(ctx, sectionB, partition.FromInts(13, 33), time.Minute, i.clbk)
				So(err, ShouldBeNil)
				So(i.deadline(), ShouldEqual, clock.Now(ctx).Add(time.Minute))
				So(i.parts(), ShouldResemble, partition.SortedPartitions{partition.FromInts(13, 33)})
			})

			Convey("WithLease obeys context deadline", func() {
				ctx, cancel := clock.WithTimeout(ctx, time.Second)
				defer cancel()
				i := inLease{}
				err = lessor.WithLease(ctx, sectionB, partition.FromInts(13, 33), time.Minute, i.clbk)
				So(err, ShouldBeNil)
				So(i.deadline(), ShouldEqual, clock.Now(ctx).Add(time.Second))
			})
		})

		Convey("only active", func() {
			active, expired, err := loadAll(ctx, sectionA)
			So(err, ShouldBeNil)
			So(sortLeases(active...), ShouldResemble, sortLeases(l1, l2, l3))
			So(len(expired), ShouldEqual, 0)

			i := inLease{}
			err = lessor.WithLease(ctx, sectionA, partition.FromInts(13, 33), time.Minute, i.clbk)
			So(err, ShouldBeNil)
			So(i.parts(), ShouldResemble, partition.SortedPartitions{
				partition.FromInts(15, 20),
				partition.FromInts(25, 30),
			})

			Convey("WithLease may lease no partitions", func() {
				i := inLease{}
				err = lessor.WithLease(ctx, sectionA, partition.FromInts(13, 15), time.Minute, i.clbk)
				So(err, ShouldBeNil)
				i.assertCalled()
				So(len(i.parts()), ShouldEqual, 0)
			})
		})

		tclock.Add(90 * time.Second)
		Convey("active and expired", func() {
			active, expired, err := loadAll(ctx, sectionA)
			So(err, ShouldBeNil)
			So(sortLeases(active...), ShouldResemble, sortLeases(l2, l3))
			So(sortLeases(expired...), ShouldResemble, sortLeases(l1))

			i := inLease{}
			err = lessor.WithLease(ctx, sectionA, partition.FromInts(13, 33), time.Minute, i.clbk)
			So(err, ShouldBeNil)
			So(i.parts(), ShouldResemble, partition.SortedPartitions{
				partition.FromInts(13, 20),
				partition.FromInts(25, 30),
			})
		})

		tclock.Add(90 * time.Second)
		Convey("only expired", func() {
			active, expired, err := loadAll(ctx, sectionA)
			So(err, ShouldBeNil)
			So(len(active), ShouldEqual, 0)
			So(sortLeases(expired...), ShouldResemble, sortLeases(l1, l2, l3))

			i := inLease{}
			err = lessor.WithLease(ctx, sectionA, partition.FromInts(13, 33), time.Minute, i.clbk)
			So(err, ShouldBeNil)
			So(i.parts(), ShouldResemble, partition.SortedPartitions{partition.FromInts(13, 33)})
		})
	})
}

func sortLeases(ls ...*lease) []*lease {
	sort.Slice(ls, func(i, j int) bool { return ls[i].Id < ls[j].Id })
	return ls
}

// inLease captures WithLeaseCB args to assert on in test.
type inLease struct {
	ctx context.Context
	sp  partition.SortedPartitions
}

func (i *inLease) clbk(ctx context.Context, sp partition.SortedPartitions) {
	if i.ctx != nil {
		panic("called twice")
	}
	i.ctx = ctx
	i.sp = sp
}

func (i *inLease) assertCalled() {
	if i.ctx == nil {
		panic("clbk never called")
	}
}

func (i *inLease) deadline() time.Time {
	i.assertCalled()
	d, ok := i.ctx.Deadline()
	if !ok {
		panic("deadline not set")
	}
	return d
}

func (i *inLease) parts() partition.SortedPartitions {
	i.assertCalled()
	return i.sp
}
