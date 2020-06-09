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
	"sort"
	"sync"
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/ttq/internal"

	"github.com/google/uuid"
	. "github.com/smartystreets/goconvey/convey"
)

func mkPartition(low, high int64) *internal.Partition {
	return internal.PartitionFromInts(low, high)
}

type ramDB struct {
	mu     sync.RWMutex
	rems   map[string]*internal.Reminder
	leases map[int]map[string]*internal.Lease
}

func (db *ramDB) SaveReminder(_ context.Context, r *internal.Reminder) error {
	db.mu.Lock()
	db.rems[r.ID] = r
	db.mu.Unlock()
	return nil
}

func (db *ramDB) DeleteReminder(_ context.Context, r *internal.Reminder) error {
	db.mu.Lock()
	for id, _ := range db.rems {
		if id == r.ID {
			delete(db.rems, id)
			break
		}
	}
	db.mu.Unlock()
	return nil
}

func (db *ramDB) FetchReminderMeta(ctx context.Context, low string, high string, limit int) ([]*internal.Reminder, error) {
	db.mu.RLock()
	var ids []string
	for id, _ := range db.rems {
		if low <= id && id < high {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	rs := make([]*internal.Reminder, len(ids))
	for i, id := range ids {
		r := db.rems[id]
		rs[i] = &internal.Reminder{ID: r.ID, FreshUntil: r.FreshUntil}
	}
	db.mu.RUnlock()
	return rs, nil
}

func (db *ramDB) FetchReminderPayloads(_ context.Context, _ []*internal.Reminder) ([]*internal.Reminder, error) {
	panic("not implemented") // TODO: Implement
}

func (db *ramDB) RunInTransaction(ctx context.Context, f func(context.Context) error) error {
	return f(ctx)
}

func (db *ramDB) LoadLeases(_ context.Context, sh int) (ls []*internal.Lease, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.leases == nil {
		return
	}
	lm, ok := db.leases[sh]
	if !ok {
		return
	}
	for _, l := range lm {
		ls = append(ls, &internal.Lease{
			Parts:     l.Parts,
			ExpiresAt: l.ExpiresAt,
			Impl:      l.Impl,
		})
	}
	return
}

type metaL struct {
	sh  int
	lid string
}

func (db *ramDB) SaveLease(_ context.Context, l *internal.Lease, sh int) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.leases == nil {
		db.leases = map[int]map[string]*internal.Lease{}
	}
	if _, ok := db.leases[sh]; !ok {
		db.leases[sh] = map[string]*internal.Lease{}
	}
	u, _ := uuid.NewRandom()
	m := &metaL{sh, u.String()}
	l.Impl = m
	db.leases[sh][m.lid] = l
	return nil
}

func (db *ramDB) ReturnLease(ctx context.Context, l *internal.Lease) error {
	return db.DeleteExpiredLeases(ctx, []*internal.Lease{l})
}

func (db *ramDB) DeleteExpiredLeases(_ context.Context, exp []*internal.Lease) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.leases == nil {
		return nil
	}
	for _, l := range exp {
		m := l.Impl.(*metaL)
		mp, ok := db.leases[m.sh]
		if !ok {
			continue
		}
		delete(mp, m.lid)
	}
	return nil
}

func (db *ramDB) Kind() string {
	return "ramdb"
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

		t := New("", "", nil, &ramDB{})

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
