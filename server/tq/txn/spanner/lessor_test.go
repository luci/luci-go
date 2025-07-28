// Copyright 2021 The LUCI Authors.
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

package spanner

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/spantest"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq/internal/partition"
)

const (
	sectionA = "sectionA"
	sectionB = "sectionB"
)

func TestLeasing(t *testing.T) {
	ftt.Run("leasing works", t, func(t *ftt.Test) {
		ctx := spantest.SpannerTestContext(t, cleanupDatabase)
		now := clock.Now(ctx).UTC()
		ctx, tclock := testclock.UseTime(ctx, now)
		lessor := spanLessor{}

		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			l := save(ctx, sectionA, now.Add(time.Minute), nil, 1)
			assert.Loosely(t, l.LeaseID, should.BeZero)

			all, err := loadAll(ctx, sectionA)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(all), should.BeZero)
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		var l1, l2, l3 *lease
		// Save 3 leases with 1, 2, 3 minutes expiry, respectively.
		_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			l1 = save(ctx, sectionA, now.Add(time.Minute), partition.SortedPartitions{
				partition.FromInts(10, 15),
			}, 0)
			l2 = save(ctx, sectionA, now.Add(2*time.Minute), partition.SortedPartitions{
				partition.FromInts(20, 25),
			}, 1)
			l3 = save(ctx, sectionA, now.Add(3*time.Minute), partition.SortedPartitions{
				partition.FromInts(30, 35),
			}, 2)
			l1.parts = nil
			l2.parts = nil
			l3.parts = nil
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run("diff shard", func(t *ftt.Test) {
			all, err := loadAll(span.Single(ctx), sectionB)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(all), should.BeZero)

			t.Run("WithLease sets context deadline at lease expiry", func(t *ftt.Test) {
				i := inLease{}
				err = lessor.WithLease(ctx, sectionB, partition.FromInts(13, 33), time.Minute, i.clbk)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, i.deadline(), should.Match(clock.Now(ctx).Add(time.Minute)))
				assert.Loosely(t, i.parts(), should.Match(partition.SortedPartitions{partition.FromInts(13, 33)}))
			})

			t.Run("WithLease obeys context deadline", func(t *ftt.Test) {
				ctx, cancel := clock.WithTimeout(ctx, time.Second)
				defer cancel()
				i := inLease{}
				err = lessor.WithLease(ctx, sectionB, partition.FromInts(13, 33), time.Minute, i.clbk)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, i.deadline(), should.Match(clock.Now(ctx).Add(time.Second)))
			})
		})

		t.Run("only active", func(t *ftt.Test) {
			all, err := loadAll(span.Single(ctx), sectionA)
			assert.Loosely(t, err, should.BeNil)
			active, expired := activeAndExpired(ctx, all)
			assert.Loosely(t, sortLeases(active...), should.Match(sortLeases(l1, l2, l3)))
			assert.Loosely(t, len(expired), should.BeZero)

			i := inLease{}
			err = lessor.WithLease(ctx, sectionA, partition.FromInts(13, 33), time.Minute, i.clbk)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, i.parts(), should.Match(partition.SortedPartitions{
				partition.FromInts(15, 20),
				partition.FromInts(25, 30),
			}))

			t.Run("WithLease may lease no partitions", func(t *ftt.Test) {
				i := inLease{}
				err = lessor.WithLease(ctx, sectionA, partition.FromInts(13, 15), time.Minute, i.clbk)
				assert.Loosely(t, err, should.BeNil)
				i.assertCalled()
				assert.Loosely(t, len(i.parts()), should.BeZero)
			})
		})

		tclock.Add(90 * time.Second)
		t.Run("active and expired", func(t *ftt.Test) {
			all, err := loadAll(span.Single(ctx), sectionA)
			assert.Loosely(t, err, should.BeNil)
			active, expired := activeAndExpired(ctx, all)
			assert.Loosely(t, sortLeases(active...), should.Match(sortLeases(l2, l3)))
			assert.Loosely(t, sortLeases(expired...), should.Match(sortLeases(l1)))

			i := inLease{}
			err = lessor.WithLease(ctx, sectionA, partition.FromInts(13, 33), time.Minute, i.clbk)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, i.parts(), should.Match(partition.SortedPartitions{
				partition.FromInts(13, 20),
				partition.FromInts(25, 30),
			}))
		})

		tclock.Add(90 * time.Second)
		t.Run("only expired", func(t *ftt.Test) {
			all, err := loadAll(span.Single(ctx), sectionA)
			assert.Loosely(t, err, should.BeNil)
			active, expired := activeAndExpired(ctx, all)
			assert.Loosely(t, len(active), should.BeZero)
			assert.Loosely(t, sortLeases(expired...), should.Match(sortLeases(l1, l2, l3)))

			i := inLease{}
			err = lessor.WithLease(ctx, sectionA, partition.FromInts(13, 33), time.Minute, i.clbk)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, i.parts(), should.Match(partition.SortedPartitions{partition.FromInts(13, 33)}))
		})
	})
}

func sortLeases(ls ...*lease) []*lease {
	sort.Slice(ls, func(i, j int) bool { return ls[i].LeaseID < ls[j].LeaseID })
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
