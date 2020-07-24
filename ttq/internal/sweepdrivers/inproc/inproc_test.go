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

package inproc

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/ttq"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/ttq/internal"
	ttqtesting "go.chromium.org/luci/ttq/internal/testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSweeper(t *testing.T) {
	t.Parallel()

	Convey("Sweeper works", t, func() {
		epoch := testclock.TestRecentTimeLocal
		ctx := cryptorand.MockForTest(gaetesting.TestingContext(), 111)
		ctx, tclock := testclock.UseTime(ctx, epoch)
		if testing.Verbose() {
			ctx = gologger.StdConfig.Use(ctx)
			ctx = logging.SetLevel(ctx, logging.Debug)
		}

		db := internal.FakeDB{}
		userCT := ttqtesting.FakeCloudTasks{}
		impl := internal.Impl{DB: &db, TasksClient: &userCT, Options: ttq.Options{
			Shards:              2,
			SecondaryScanShards: 2,
			TasksPerScan:        10,
		}}

		sw := NewSweeper(&impl, nil)

		var cancelSweeping func()
		tclock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			switch {
			case testclock.HasTags(t, "coordinator"):
				logging.Debugf(ctx, "unblocking dispatcher.Channel")
				tclock.Add(d)
			case testclock.HasTags(t, "ttq-loop"):
				logging.Debugf(ctx, "cancelSweeping")
				cancelSweeping()
			}
		})
		sweepOnce := func() {
			innerCtx, cancel := context.WithCancel(ctx)
			cancelSweeping = cancel
			// cancelSweeping should be called by SetTimerCallback above,
			// but in case of bad test or code under test, call it here, too.
			defer cancelSweeping()
			sw.SweepContinuously(innerCtx)
		}

		Convey("noop", func() {
			sweepOnce()
			So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 0)
		})

		Convey("Some reminders", func() {
			tclock.Set(epoch)
			_, err := impl.AddTask(ctx, genUserTask(ctx)) // don't call postProcess on this one.
			So(err, ShouldBeNil)
			tclock.Add(time.Hour) // let Reminder for the first user task turn stale.
			pp2, err := impl.AddTask(ctx, genUserTask(ctx))
			So(err, ShouldBeNil)
			So(len(db.AllReminders()), ShouldEqual, 2) // 1 fresh + 1 stale.

			// Sweep.
			sweepOnce()
			So(len(db.AllReminders()), ShouldEqual, 1) // 1 fresh remaining.
			So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 1)

			pp2(ctx)
			So(len(db.AllReminders()), ShouldEqual, 0)
			So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 1)
		})

		Convey("3 stale Reminders", func() {
			for i := 0; i < 3; i++ {
				_, err := impl.AddTask(ctx, genUserTask(ctx)) // don't call postProcess.
				So(err, ShouldBeNil)
			}
			tclock.Add(time.Hour) // turn all Reminders stale.
			So(len(db.AllReminders()), ShouldEqual, 3)

			Convey("Extra scan", func() {
				// Ensures first scan will have 1 follow up scan.
				impl.Options.Shards = 1
				impl.Options.TasksPerScan = 2
				sweepOnce()
				So(len(db.AllReminders()), ShouldEqual, 0)
				So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 3)
			})

			Convey("User task creation error", func() {
				impl.Options.Shards = 2
				So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 0)
				userCT.FakeCreateTaskErrors(
					status.Errorf(codes.Unavailable, "please retry"),
					status.Errorf(codes.InvalidArgument, "user error"),
				)
				sweepOnce()
				// 3 tried, 1 of which failed permanently, ...
				So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 3)
				// ... and 1 should be retried.
				So(len(db.AllReminders()), ShouldEqual, 1)

				sweepOnce()
				So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 1)
				So(len(db.AllReminders()), ShouldEqual, 0)
			})
		})
	})
}

func genUserTask(ctx context.Context) *taskspb.CreateTaskRequest {
	return &taskspb.CreateTaskRequest{
		Parent: "projects/example-project/locations/us-central1/queues/q",
		Task: &taskspb.Task{
			MessageType: &taskspb.Task_HttpRequest{HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_GET,
				Url:        "https://example.com/user/exec",
			}},
			ScheduleTime: google.NewTimestamp(clock.Now(ctx).Add(time.Minute)),
		},
	}
}
