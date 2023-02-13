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

	"go.chromium.org/luci/server/tq/internal/lessor"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"
	"go.chromium.org/luci/server/tq/internal/tqpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDistributed(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
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

		Convey("No reminders", func() {
			err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
			So(err, ShouldBeNil)
			So(enqueued, ShouldBeEmpty)
			So(sub.req, ShouldBeEmpty)
		})

		Convey("Two reminders, one still fresh", func() {
			So(db.SaveReminder(ctx, mkReminder(77, true, "r1")), ShouldBeNil)
			So(db.SaveReminder(ctx, mkReminder(111, false, "r2")), ShouldBeNil)

			err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
			So(err, ShouldBeNil)
			So(enqueued, ShouldBeEmpty)
			So(sub.req, ShouldHaveLength, 1)
			So(sub.req[0].CreateTaskRequest.Parent, ShouldEqual, "r2")

			So(db.AllReminders(), ShouldHaveLength, 1)
		})

		Convey("With follow up tasks", func() {
			for i := 0; i < 64; i++ {
				So(db.SaveReminder(ctx, mkReminder(i, false, "")), ShouldBeNil)
			}

			tasksPerScan = 32
			secondaryScanShards = 2

			err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
			So(err, ShouldBeNil)

			sort.Slice(enqueued, func(i, j int) bool {
				return enqueued[i].Partition < enqueued[j].Partition
			})
			So(enqueued, ShouldHaveLength, 2)
			So(enqueued[0], ShouldResembleProto, sweepTask(32, 144, 1))
			So(enqueued[1], ShouldResembleProto, sweepTask(144, 256, 1))

			So(sub.req, ShouldHaveLength, 32)           // submitted up to the limit
			So(db.AllReminders(), ShouldHaveLength, 32) // the rest are still there
		})

		Convey("With batching", func() {
			for i := 0; i < 100; i++ {
				So(db.SaveReminder(ctx, mkReminder(i, false, "")), ShouldBeNil)
			}

			Convey("Success", func() {
				err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
				So(err, ShouldBeNil)
				So(enqueued, ShouldBeEmpty)
				So(sub.req, ShouldHaveLength, 100)
				So(db.AllReminders(), ShouldBeEmpty) // submitted everything
			})

			Convey("Transient errors", func() {
				sub.err = status.Errorf(codes.Internal, "boo")

				err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
				So(err, ShouldNotBeNil)
				So(enqueued, ShouldBeEmpty)
				So(sub.req, ShouldHaveLength, 100)           // tried
				So(db.AllReminders(), ShouldHaveLength, 100) // but failed and kept reminders
			})
		})

		Convey("Partial lease", func() {
			for i := 0; i < 100; i++ {
				So(db.SaveReminder(ctx, mkReminder(i, false, fmt.Sprintf("%04d", i))), ShouldBeNil)
			}

			lessor.leased = partition.SortedPartitions{
				partition.FromInts(0, 5),   // [0, 5)
				partition.FromInts(15, 16), // single 15
				partition.FromInts(20, 25), // [20, 25)
			}

			err := dist.ExecSweepTask(ctx, sweepTask(0, 256, 0))
			So(err, ShouldBeNil)
			So(enqueued, ShouldBeEmpty)
			So(sub.req, ShouldHaveLength, 5+1+5)

			submitted := []string{}
			for _, r := range sub.req {
				submitted = append(submitted, r.CreateTaskRequest.Parent)
			}
			sort.Strings(submitted)
			So(submitted, ShouldResemble, []string{
				"0000", "0001", "0002", "0003", "0004",
				"0015",
				"0020", "0021", "0022", "0023", "0024",
			})
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
