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

		sw := Sweeper{Impl: &impl, Opts: &Options{
			ScanInterval:                            time.Minute,
			MaxConcurrentScansPerShard:              10,
			MaxScansBacklog:                         128,
			MaxConcurrentPostProcessBatchesPerShard: 10,
			BatchPostProcessSize:                    50,
		}}

		Convey("Idle efficiently", func() {
			tclock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				if testclock.HasTags(t, "shard-loop") {
					tclock.Add(d)
				}
			})
			started := clock.Now(ctx)
			impl.Options.Shards = 1
			sw.Opts.ScanInterval = time.Minute
			sw.shouldContinue = func(_ int, iteration int64) bool { return iteration < 3 }
			sw.SweepContinuously(ctx)
			ended := clock.Now(ctx)
			So(ended.Sub(started), ShouldEqual, 2*time.Minute)
			So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 0)
		})

		Convey("Cancelable", func() {
			tclock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				if testclock.HasTags(t, "shard-loop") {
					tclock.Add(d)
				}
			})

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			sw.shouldContinue = func(shard int, iteration int64) bool {
				if shard == 1 && iteration == 3 {
					cancel()
				}
				return true
			}
			// This should not hang.
			sw.SweepContinuously(ctx)
		})

		sweepOnce := func(ctx context.Context) {
			sw.shouldContinue = func(_ int, iteration int64) bool { return false }
			sw.SweepContinuously(ctx)
		}
		tclock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if testclock.HasTags(t, "coordinator") {
				tclock.Add(d)
			}
		})

		Convey("Process some Reminders", func() {
			tclock.Set(epoch)
			_, err := impl.AddTask(ctx, genUserTask(ctx)) // don't call postProcess on this one.
			So(err, ShouldBeNil)
			tclock.Add(time.Hour) // let Reminder for the first user task turn stale.
			pp2, err := impl.AddTask(ctx, genUserTask(ctx))
			So(err, ShouldBeNil)
			So(len(db.AllReminders()), ShouldEqual, 2) // 1 fresh + 1 stale.

			sweepOnce(ctx)
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
				sweepOnce(ctx)
				So(len(db.AllReminders()), ShouldEqual, 0)
				So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 3)
			})

			Convey("User task creation error", func() {
				So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 0)
				userCT.FakeCreateTaskErrors(
					status.Errorf(codes.Unavailable, "please retry"),
					status.Errorf(codes.InvalidArgument, "user error"),
				)
				sweepOnce(ctx)
				// 3 tried, 1 of which failed permanently, ...
				So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 3)
				// ... and 1 should be retried.
				So(len(db.AllReminders()), ShouldEqual, 1)

				sweepOnce(ctx)
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
