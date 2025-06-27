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

package sweep

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/tq/internal/lessor"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"
	"go.chromium.org/luci/server/tq/internal/tqpb"
)

func TestDistributed(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		var epoch = testclock.TestRecentTimeLocal
		const keySpaceBytes = 2

		ctx, _ := testclock.UseTime(context.Background(), epoch)
		sub := &submitter{}

		db := &testutil.FakeDB{}
		ctx = db.Inject(ctx)

		lessor := &fakeLessor{}
		ctx = lessor.Inject(ctx)

		var mu sync.Mutex
		var enqueued []*tqpb.SweepTask

		dist := Distributed{
			EnqueueSweepTask: func(_ context.Context, task *tqpb.SweepTask) error {
				mu.Lock()
				enqueued = append(enqueued, task)
				mu.Unlock()
				return nil
			},
			Submitter: sub,
		}

		tasksPerScan := 10000
		secondaryScanShards := 2

		sweepTask := func(low, high int64, level int) *tqpb.SweepTask {
			return &tqpb.SweepTask{
				Db:                  db.Kind(),
				Partition:           partition.FromInts(low, high).String(),
				LessorId:            "fakeLessor",
				LeaseSectionId:      "section_id",
				ShardCount:          1111, // not really used
				ShardIndex:          111,  // not really used
				Level:               int32(level),
				KeySpaceBytes:       keySpaceBytes, // for simpler total space
				TasksPerScan:        int32(tasksPerScan),
				SecondaryScanShards: int32(secondaryScanShards),
			}
		}

		mkReminder := func(id int, fresh bool, body string) *reminder.Reminder {
			rem := &reminder.Reminder{}
			rem.ID, _ = partition.FromInts(int64(id), int64(id)+1).QueryBounds(keySpaceBytes)
			if fresh {
				rem.FreshUntil = epoch.Add(5 * time.Second)
			} else {
				rem.FreshUntil = epoch.Add(-5 * time.Second)
			}
			rem.AttachPayload(&reminder.Payload{
				CreateTaskRequest: &taskspb.CreateTaskRequest{Parent: body},
			})
			return rem
		}

		t.Run("No reminders", func(t *ftt.Test) {
			err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, enqueued, should.BeEmpty)
			assert.Loosely(t, sub.req, should.BeEmpty)
		})

		t.Run("Two reminders, one still fresh", func(t *ftt.Test) {
			assert.Loosely(t, db.SaveReminder(ctx, mkReminder(77, true, "r1")), should.BeNil)
			assert.Loosely(t, db.SaveReminder(ctx, mkReminder(111, false, "r2")), should.BeNil)

			err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, enqueued, should.BeEmpty)
			assert.Loosely(t, sub.req, should.HaveLength(1))
			assert.Loosely(t, sub.req[0].CreateTaskRequest.Parent, should.Equal("r2"))

			assert.Loosely(t, db.AllReminders(), should.HaveLength(1))
		})

		t.Run("With follow up tasks", func(t *ftt.Test) {
			for i := range 64 {
				assert.Loosely(t, db.SaveReminder(ctx, mkReminder(i, false, "")), should.BeNil)
			}

			tasksPerScan = 32
			secondaryScanShards = 2

			err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
			assert.Loosely(t, err, should.BeNil)

			sort.Slice(enqueued, func(i, j int) bool {
				return enqueued[i].Partition < enqueued[j].Partition
			})
			assert.Loosely(t, enqueued, should.HaveLength(2))
			assert.Loosely(t, enqueued[0], should.Match(sweepTask(32, 144, 1)))
			assert.Loosely(t, enqueued[1], should.Match(sweepTask(144, 256, 1)))

			assert.Loosely(t, sub.req, should.HaveLength(32))           // submitted up to the limit
			assert.Loosely(t, db.AllReminders(), should.HaveLength(32)) // the rest are still there
		})

		t.Run("With batching", func(t *ftt.Test) {
			for i := range 100 {
				assert.Loosely(t, db.SaveReminder(ctx, mkReminder(i, false, "")), should.BeNil)
			}

			t.Run("Success", func(t *ftt.Test) {
				err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, enqueued, should.BeEmpty)
				assert.Loosely(t, sub.req, should.HaveLength(100))
				assert.Loosely(t, db.AllReminders(), should.BeEmpty) // submitted everything
			})

			t.Run("Transient errors", func(t *ftt.Test) {
				sub.err = status.Errorf(codes.Internal, "boo")

				err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, enqueued, should.BeEmpty)
				assert.Loosely(t, sub.req, should.HaveLength(100))           // tried
				assert.Loosely(t, db.AllReminders(), should.HaveLength(100)) // but failed and kept reminders
			})
		})

		t.Run("Partial lease", func(t *ftt.Test) {
			for i := range 100 {
				assert.Loosely(t, db.SaveReminder(ctx, mkReminder(i, false, fmt.Sprintf("%04d", i))), should.BeNil)
			}

			lessor.leased = partition.SortedPartitions{
				partition.FromInts(0, 5),   // [0, 5)
				partition.FromInts(15, 16), // single 15
				partition.FromInts(20, 25), // [20, 25)
			}

			err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, enqueued, should.BeEmpty)
			assert.Loosely(t, sub.req, should.HaveLength(5+1+5))

			submitted := []string{}
			for _, r := range sub.req {
				submitted = append(submitted, r.CreateTaskRequest.Parent)
			}
			sort.Strings(submitted)
			assert.Loosely(t, submitted, should.Match([]string{
				"0000", "0001", "0002", "0003", "0004",
				"0015",
				"0020", "0021", "0022", "0023", "0024",
			}))
		})
	})
}

type submitter struct {
	cb  func(req *reminder.Payload) error
	err error
	mu  sync.Mutex
	req []*reminder.Payload
}

func (s *submitter) Submit(ctx context.Context, req *reminder.Payload) error {
	if s.cb != nil {
		if err := s.cb(req); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.req = append(s.req, req)
	s.mu.Unlock()
	return s.err
}

type fakeLessor struct {
	leased partition.SortedPartitions
}

func (f *fakeLessor) WithLease(ctx context.Context, sectionID string, part *partition.Partition, dur time.Duration, cb lessor.WithLeaseCB) error {
	l := f.leased
	if l == nil {
		l = partition.SortedPartitions{part}
	}
	cb(ctx, l)
	return nil
}

var fakeLessorKey = "fakeLessor"

func (f *fakeLessor) Inject(ctx context.Context) context.Context {
	return context.WithValue(ctx, &fakeLessorKey, f)
}

func init() {
	lessor.Register("fakeLessor", func(ctx context.Context) (lessor.Lessor, error) {
		l, _ := ctx.Value(&fakeLessorKey).(*fakeLessor)
		return l, nil
	})
}
