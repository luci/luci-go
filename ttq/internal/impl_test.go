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
	"sync"
	"testing"
	"time"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/ttq"
	"go.chromium.org/luci/ttq/internal/partition"

	"github.com/googleapis/gax-go"
	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMakeReminder(t *testing.T) {
	t.Parallel()

	Convey("makeReminder", t, func() {
		ctx := cryptorand.MockForTest(context.Background(), 111)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
		req := taskspb.CreateTaskRequest{
			Parent: "projects/example-project/locations/us-central1/queues/q",
			Task: &taskspb.Task{
				MessageType: &taskspb.Task_HttpRequest{HttpRequest: &taskspb.HttpRequest{
					HttpMethod: taskspb.HttpMethod_GET,
					Url:        "https://example.com/user/exec",
				}},
				ScheduleTime: google.NewTimestamp(clock.Now(ctx).Add(time.Minute)),
			},
		}

		Convey("typical request with deadline", func() {
			deadline := testclock.TestRecentTimeLocal.Add(5 * time.Second)
			reqCtx, cancel := clock.WithDeadline(ctx, deadline)
			defer cancel()
			r, clonedReq, err := makeReminder(reqCtx, &req)
			So(err, ShouldBeNil)
			So(r.Id, ShouldResemble, "1408d4aaccbdf3f4b03972992f179ce4")
			So(len(r.Id), ShouldEqual, 2*keySpaceBytes)
			So(r.FreshUntil, ShouldEqual, deadline.UTC())
			So(clonedReq.Task.Name, ShouldResemble,
				"projects/example-project/locations/us-central1/queues/q/tasks/1408d4aaccbdf3f4b03972992f179ce4")

			deserialized, err := r.createTaskRequest()
			So(err, ShouldBeNil)
			So(clonedReq, ShouldResembleProto, deserialized)

			clonedReq.Task.Name = ""
			So(clonedReq, ShouldResembleProto, &req)
		})

		Convey("reasonable FreshUntil without deadline", func() {
			r, _, err := makeReminder(ctx, &req)
			So(err, ShouldBeNil)
			So(r.FreshUntil, ShouldEqual, testclock.TestRecentTimeLocal.Add(happyPathMaxDuration).UTC())
		})

		Convey("named tasks not supported", func() {
			req.Task.Name = req.Parent + "/tasks/deadbeef"
			_, _, err := makeReminder(ctx, &req)
			So(err, ShouldErrLike, "Named tasks are not supported")
		})
	})
}

func TestAddTask(t *testing.T) {
	t.Parallel()

	Convey("AddTask works", t, func() {
		ctx := cryptorand.MockForTest(context.Background(), 111)
		ctx, tclock := testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		latestOf := func(m types.Metric, fieldVals ...interface{}) interface{} {
			return tsmon.GetState(ctx).Store().Get(ctx, m, time.Time{}, fieldVals)
		}

		req := taskspb.CreateTaskRequest{
			Parent: "projects/example-project/locations/us-central1/queues/q",
			Task: &taskspb.Task{
				MessageType: &taskspb.Task_HttpRequest{HttpRequest: &taskspb.HttpRequest{
					HttpMethod: taskspb.HttpMethod_GET,
					Url:        "https://example.com/user/exec",
				}},
				ScheduleTime: google.NewTimestamp(clock.Now(ctx).Add(time.Minute)),
			},
		}
		opts := ttq.Options{
			Queue:   "projects/example-project/locations/us-central1/queues/ttq",
			BaseURL: "https://example.com/ttq",
		}
		So(opts.Validate(), ShouldBeNil)
		db := FakeDB{}
		fakeCT := fakeCloudTasks{}
		impl := Impl{Options: opts, DB: &db, TasksClient: &fakeCT}

		Convey("with deadline", func() {
			pp, err := impl.AddTask(ctx, &req)
			So(err, ShouldBeNil)
			So(pp, ShouldNotBeNil)
			So(len(db.reminders), ShouldEqual, 1)

			Convey("no time to complete happy path", func() {
				for _, r := range db.reminders {
					tclock.Set(r.FreshUntil.Add(time.Second))
				}
				pp(ctx) // noop now
				So(len(fakeCT.calls), ShouldEqual, 0)
				So(len(db.reminders), ShouldEqual, 1)
				So(latestOf(metricPostProcessedAttempts, "OK", "happy", "FakeDB"), ShouldBeNil)
			})

			Convey("happy path", func() {
				pp(ctx)
				So(len(db.reminders), ShouldEqual, 0)
				So(len(fakeCT.calls), ShouldEqual, 1)
				So(latestOf(metricPostProcessedAttempts, "OK", "happy", "FakeDB"), ShouldEqual, 1)
				So(latestOf(metricPostProcessedDurationsMS, "OK", "happy", "FakeDB").(*distribution.Distribution).Count(), ShouldEqual, 1)

				Convey("calling 2nd time is a noop", func() {
					pp(ctx)
					So(len(fakeCT.calls), ShouldEqual, 1)
					So(latestOf(metricPostProcessedAttempts, "OK", "happy", "FakeDB"), ShouldEqual, 1)
				})
			})
			Convey("happy path transient task creation error", func() {
				fakeCT.errs = []error{status.Errorf(codes.Unavailable, "please retry")}
				pp(ctx)
				So(len(db.reminders), ShouldEqual, 1)
				So(latestOf(metricPostProcessedAttempts, "fail-task", "happy", "FakeDB"), ShouldEqual, 1)
			})
			Convey("happy path permanent task creation error", func() {
				fakeCT.errs = []error{status.Errorf(codes.InvalidArgument, "forever")}
				pp(ctx)
				So(latestOf(metricPostProcessedAttempts, "ignored-bad-task", "happy", "FakeDB"), ShouldEqual, 1)
				So(latestOf(metricTasksCreated, "InvalidArgument", "happy", "FakeDB"), ShouldEqual, 1)
				// Must remove reminder despite the error.
				So(len(db.reminders), ShouldEqual, 0)
			})
		})
	})
}

func TestLeaseHelpers(t *testing.T) {
	t.Parallel()

	Convey("Lease Helpers", t, func() {

		Convey("onlyLeased", func() {
			reminders := []*Reminder{
				// Each key be exactly 2*keySpaceBytes chars long.
				&Reminder{Id: "00000000000000000000000000000001"},
				&Reminder{Id: "00000000000000000000000000000005"},
				&Reminder{Id: "00000000000000000000000000000009"},
				&Reminder{Id: "0000000000000000000000000000000f"}, // ie 15
			}
			leased := partition.SortedPartitions{partition.FromInts(5, 9)}
			So(onlyLeased(reminders, leased), ShouldResemble, []*Reminder{
				&Reminder{Id: "00000000000000000000000000000005"},
			})
		})
	})
}

// helpers

type fakeCloudTasks struct {
	mu    sync.Mutex
	errs  []error
	calls []*taskspb.CreateTaskRequest
}

func (f *fakeCloudTasks) CreateTask(_ context.Context, req *taskspb.CreateTaskRequest, _ ...gax.CallOption) (*taskspb.Task, error) {
	// NOTE: actual implementation returns the created Task, but the TTQ library
	// doesn't care.
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	if len(f.errs) > 0 {
		var err error
		f.errs, err = f.errs[1:], f.errs[0]
		return nil, err
	}
	return nil, nil
}
