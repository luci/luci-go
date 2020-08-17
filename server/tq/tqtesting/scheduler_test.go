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

package tqtesting

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestScheduler(t *testing.T) {
	t.Parallel()

	Convey("With scheduler", t, func() {
		var epoch = testclock.TestRecentTimeUTC

		ctx, tc := testclock.UseTime(context.Background(), epoch)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if testclock.HasTags(t, ClockTag) {
				tc.Add(d)
			}
		})

		exec := testExecutor{
			ctx: ctx,
			ch:  make(chan *Task, 1000), // ~= infinite buffer
		}
		sched := Scheduler{Executor: &exec}

		run := func(untilCount int) {
			ctx, cancel := context.WithCancel(ctx)

			done := make(chan struct{})
			go func() {
				defer close(done)
				sched.Run(ctx)
			}()

			exec.waitForTasks(untilCount)
			cancel()
			<-done

			So(sched.Tasks(), ShouldBeEmpty)
		}

		enqueue := func(payload, name string, eta time.Time) codes.Code {
			req := &taskspb.CreateTaskRequest{
				Parent: "projects/zzz/locations/zzz/queues/zzz",
				Task: &taskspb.Task{
					MessageType: &taskspb.Task_HttpRequest{
						HttpRequest: &taskspb.HttpRequest{
							Url: payload,
						},
					},
				},
			}
			if name != "" {
				req.Task.Name = req.Parent + "/tasks/" + name
			}
			if !eta.IsZero() {
				req.Task.ScheduleTime = timestamppb.New(eta)
			}
			return status.Code(sched.Submit(ctx, req, nil))
		}

		Convey("One by one tasks", func() {
			So(enqueue("1", "name", time.Time{}), ShouldEqual, codes.OK)
			So(enqueue("2", "name", time.Time{}), ShouldEqual, codes.AlreadyExists)
			So(enqueue("3", "", time.Time{}), ShouldEqual, codes.OK)
			So(enqueue("4", "", time.Time{}), ShouldEqual, codes.OK)

			run(3)

			So(orderByPayload(exec.tasks), ShouldResemble, []string{"1", "3", "4"})
		})

		Convey("Task chain", func() {
			exec.execute = func(payload string, t *Task) bool {
				if len(payload) < 3 {
					enqueue(payload+".", "", time.Time{})
				}
				return true
			}
			enqueue(".", "", time.Time{})
			run(3)
			So(orderByPayload(exec.tasks), ShouldResemble, []string{".", "..", "..."})
		})

		Convey("Tasks with ETA", func() {
			now := clock.Now(ctx)
			for i := 2; i >= 0; i-- {
				enqueue(fmt.Sprintf("B %d", i), fmt.Sprintf("B %d", i), now.Add(time.Duration(i)*time.Millisecond))
				enqueue(fmt.Sprintf("A %d", i), fmt.Sprintf("A %d", i), now.Add(time.Duration(i)*time.Millisecond))
			}
			run(6)
			So(payloads(exec.tasks), ShouldResemble, []string{"A 0", "B 0", "A 1", "B 1", "A 2", "B 2"})
		})

		Convey("Retries", func() {
			var capturedTask *Task
			sched.TaskSucceeded = func(_ context.Context, t *Task) {
				capturedTask = t
			}

			exec.execute = func(payload string, t *Task) bool {
				return t.Attempts == 4
			}

			enqueue(".", "", time.Time{})
			run(4)
			So(payloads(exec.tasks), ShouldHaveLength, 4)

			So(capturedTask, ShouldNotBeNil)
			So(capturedTask.Attempts, ShouldEqual, 4)
		})

		Convey("Fails after multiple attempts", func() {
			sched.MaxAttempts = 10

			var capturedTask *Task
			sched.TaskFailed = func(_ context.Context, t *Task) {
				capturedTask = t
			}

			exec.execute = func(payload string, t *Task) bool {
				return false
			}

			enqueue(".", "", time.Time{})
			run(10)
			So(payloads(exec.tasks), ShouldHaveLength, 10)

			So(capturedTask, ShouldNotBeNil)
			So(capturedTask.Attempts, ShouldEqual, 10)
		})

		Convey("State capture", func() {
			var captured []*Task

			exec.execute = func(payload string, t *Task) bool {
				if payload == "A 1" {
					captured = sched.Tasks()
				}
				return true
			}

			now := clock.Now(ctx)
			for i := 2; i >= 0; i-- {
				enqueue(fmt.Sprintf("B %d", i), fmt.Sprintf("B %d", i), now.Add(time.Duration(i)*time.Millisecond))
				enqueue(fmt.Sprintf("A %d", i), fmt.Sprintf("A %d", i), now.Add(time.Duration(i)*time.Millisecond))
			}
			run(6)
			So(payloads(exec.tasks), ShouldResemble, []string{"A 0", "B 0", "A 1", "B 1", "A 2", "B 2"})

			So(payloads(captured), ShouldResemble, []string{"A 1", "B 1", "A 2", "B 2"})
			So(captured[0].Executing, ShouldBeTrue)
			So(captured[1].Executing, ShouldBeFalse)
		})

		Convey("Run(StopWhenDrained)", func() {
			Convey("Noop if already drained", func() {
				exec.execute = func(string, *Task) bool { panic("must no be called") }
				sched.Run(ctx, StopWhenDrained())
				So(clock.Now(ctx).Equal(epoch), ShouldBeTrue)
			})

			Convey("Stops after executing a pending task", func() {
				exec.execute = func(string, *Task) bool { return true }
				enqueue("1", "", epoch.Add(5*time.Second))
				sched.Run(ctx, StopWhenDrained())
				So(clock.Now(ctx).Sub(epoch), ShouldEqual, 5*time.Second)
				So(exec.tasks, ShouldHaveLength, 1)
			})

			Convey("Stops after draining", func() {
				exec.execute = func(title string, _ *Task) bool {
					if title == "1" {
						enqueue("2", "", clock.Now(ctx).Add(5*time.Second))
					}
					return true
				}
				enqueue("1", "", epoch.Add(5*time.Second))
				sched.Run(ctx, StopWhenDrained())
				So(clock.Now(ctx).Sub(epoch), ShouldEqual, 10*time.Second)
				So(exec.tasks, ShouldHaveLength, 2)
			})
		})
	})
}

func TestTaskList(t *testing.T) {
	t.Parallel()

	Convey("With task list", t, func() {
		var epoch = time.Unix(1442540000, 0)

		task := func(payload int, exec bool, eta int, name string) *Task {
			return &Task{
				Name:      name,
				Executing: exec,
				ETA:       epoch.Add(time.Duration(eta) * time.Second),
				Payload:   &durationpb.Duration{Seconds: int64(payload)},
			}
		}

		tl := TaskList{
			task(0, true, 3, ""),
			task(1, false, 1, ""),
			task(2, true, 2, ""),
			task(3, false, 4, ""),
			task(4, true, 5, "b"),
			task(5, true, 5, "a"),
		}

		Convey("Payloads", func() {
			So(tl.Payloads(), ShouldResembleProto, []*durationpb.Duration{
				{Seconds: 0},
				{Seconds: 1},
				{Seconds: 2},
				{Seconds: 3},
				{Seconds: 4},
				{Seconds: 5},
			})
		})

		Convey("Executing/Pending", func() {
			So(tl.Executing().Payloads(), ShouldResembleProto, []*durationpb.Duration{
				{Seconds: 0},
				{Seconds: 2},
				{Seconds: 4},
				{Seconds: 5},
			})

			So(tl.Pending().Payloads(), ShouldResembleProto, []*durationpb.Duration{
				{Seconds: 1},
				{Seconds: 3},
			})
		})

		Convey("SortByETA", func() {
			So(tl.SortByETA().Payloads(), ShouldResembleProto, []*durationpb.Duration{
				{Seconds: 2},
				{Seconds: 1},
				{Seconds: 0},
				{Seconds: 5},
				{Seconds: 4},
				{Seconds: 3},
			})
		})
	})

	Convey("TasksCollector", t, func() {
		var tl TaskList
		cb := TasksCollector(&tl)
		cb(context.Background(), &Task{})
		cb(context.Background(), &Task{})
		So(tl, ShouldHaveLength, 2)
	})
}

type testExecutor struct {
	ctx     context.Context
	execute func(payload string, t *Task) bool
	tasks   []*Task
	ch      chan *Task
}

func (exe *testExecutor) Execute(ctx context.Context, t *Task, done func(retry bool)) {
	t = t.Copy()

	success := true
	if exe.execute != nil {
		success = exe.execute(t.Task.GetHttpRequest().Url, t)
	}

	exe.tasks = append(exe.tasks, t)
	exe.ch <- t
	done(!success)
}

func (exe *testExecutor) waitForTasks(n int) {
	for ; n > 0; n-- {
		select {
		case <-exe.ch:
		case <-exe.ctx.Done():
			So("the scheduler is stuck", ShouldBeNil)
		}
	}
}

func payloads(tasks []*Task) []string {
	payloads := make([]string, len(tasks))
	for i, t := range tasks {
		payloads[i] = t.Task.GetHttpRequest().Url
	}
	return payloads
}

func orderByPayload(tasks []*Task) []string {
	sort.Slice(tasks, func(i, j int) bool {
		l, r := tasks[i].Task, tasks[j].Task
		return l.GetHttpRequest().Url < r.GetHttpRequest().Url
	})
	return payloads(tasks)
}
