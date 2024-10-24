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
	"strings"
	"sync"
	"testing"
	"time"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/tq/internal/reminder"
)

func TestScheduler(t *testing.T) {
	t.Parallel()

	ftt.Run("With scheduler", t, func(t *ftt.Test) {
		var epoch = testclock.TestRecentTimeUTC

		ctx := context.Background()
		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}

		ctx, tc := testclock.UseTime(ctx, epoch)
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

			exec.waitForTasks(t, untilCount)
			cancel()
			<-done

			assert.Loosely(t, sched.Tasks(), should.BeEmpty)
		}

		enqueue := func(payload, name string, eta time.Time, taskClassID string) codes.Code {
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
			if taskClassID == "" {
				taskClassID = "default-task-class"
			}
			return status.Code(sched.Submit(ctx, &reminder.Payload{
				TaskClass:         taskClassID,
				CreateTaskRequest: req,
			}))
		}

		t.Run("One by one tasks", func(t *ftt.Test) {
			assert.Loosely(t, enqueue("1", "name", time.Time{}, ""), should.Equal(codes.OK))
			assert.Loosely(t, enqueue("2", "name", time.Time{}, ""), should.Equal(codes.AlreadyExists))
			assert.Loosely(t, enqueue("3", "", time.Time{}, ""), should.Equal(codes.OK))
			assert.Loosely(t, enqueue("4", "", time.Time{}, ""), should.Equal(codes.OK))

			run(3)

			assert.Loosely(t, orderByPayload(exec.tasks), should.Resemble([]string{"1", "3", "4"}))
		})

		t.Run("Task chain", func(t *ftt.Test) {
			exec.execute = func(payload string, t *Task) bool {
				if len(payload) < 3 {
					enqueue(payload+".", "", time.Time{}, "")
				}
				return true
			}
			enqueue(".", "", time.Time{}, "")
			run(3)
			assert.Loosely(t, orderByPayload(exec.tasks), should.Resemble([]string{".", "..", "..."}))
		})

		t.Run("Tasks with ETA", func(t *ftt.Test) {
			now := clock.Now(ctx)
			for i := 2; i >= 0; i-- {
				enqueue(fmt.Sprintf("B %d", i), fmt.Sprintf("B %d", i), now.Add(time.Duration(i)*time.Millisecond), "")
				enqueue(fmt.Sprintf("A %d", i), fmt.Sprintf("A %d", i), now.Add(time.Duration(i)*time.Millisecond), "")
			}
			run(6)
			assert.Loosely(t, payloads(exec.tasks), should.Resemble([]string{"A 0", "B 0", "A 1", "B 1", "A 2", "B 2"}))
		})

		t.Run("Retries", func(t *ftt.Test) {
			var capturedTask *Task
			sched.TaskSucceeded = func(_ context.Context, t *Task) {
				capturedTask = t
			}

			exec.execute = func(payload string, t *Task) bool {
				return t.Attempts == 4
			}

			enqueue(".", "", time.Time{}, "")
			run(4)
			assert.Loosely(t, payloads(exec.tasks), should.HaveLength(4))

			assert.Loosely(t, capturedTask, should.NotBeNil)
			assert.Loosely(t, capturedTask.Attempts, should.Equal(4))
		})

		t.Run("Fails after multiple attempts", func(t *ftt.Test) {
			sched.MaxAttempts = 10

			var capturedTask *Task
			sched.TaskFailed = func(_ context.Context, t *Task) {
				capturedTask = t
			}

			exec.execute = func(payload string, t *Task) bool {
				return false
			}

			enqueue(".", "", time.Time{}, "")
			run(10)
			assert.Loosely(t, payloads(exec.tasks), should.HaveLength(10))

			assert.Loosely(t, capturedTask, should.NotBeNil)
			assert.Loosely(t, capturedTask.Attempts, should.Equal(10))
		})

		t.Run("State capture", func(t *ftt.Test) {
			var captured []*Task

			exec.execute = func(payload string, t *Task) bool {
				if payload == "A 1" {
					captured = sched.Tasks()
				}
				return true
			}

			now := clock.Now(ctx)
			for i := 2; i >= 0; i-- {
				enqueue(fmt.Sprintf("B %d", i), fmt.Sprintf("B %d", i), now.Add(time.Duration(i)*time.Millisecond), "")
				enqueue(fmt.Sprintf("A %d", i), fmt.Sprintf("A %d", i), now.Add(time.Duration(i)*time.Millisecond), "")
			}
			run(6)
			assert.Loosely(t, payloads(exec.tasks), should.Resemble([]string{"A 0", "B 0", "A 1", "B 1", "A 2", "B 2"}))

			assert.Loosely(t, payloads(captured), should.Resemble([]string{"A 1", "B 1", "A 2", "B 2"}))
			assert.Loosely(t, captured[0].Executing, should.BeTrue)
			assert.Loosely(t, captured[1].Executing, should.BeFalse)
		})

		t.Run("Run(StopWhenDrained)", func(t *ftt.Test) {
			t.Run("Noop if already drained", func(t *ftt.Test) {
				exec.execute = func(string, *Task) bool { panic("must no be called") }
				sched.Run(ctx, StopWhenDrained())
				assert.Loosely(t, clock.Now(ctx).Equal(epoch), should.BeTrue)
			})

			t.Run("Stops after executing a pending task", func(t *ftt.Test) {
				exec.execute = func(string, *Task) bool { return true }
				enqueue("1", "", epoch.Add(5*time.Second), "")
				sched.Run(ctx, StopWhenDrained())
				assert.Loosely(t, clock.Now(ctx).Sub(epoch), should.Equal(5*time.Second))
				assert.Loosely(t, exec.tasks, should.HaveLength(1))
			})

			t.Run("Stops after draining", func(t *ftt.Test) {
				exec.execute = func(payload string, _ *Task) bool {
					if payload == "1" {
						enqueue("2", "", clock.Now(ctx).Add(5*time.Second), "")
					}
					return true
				}
				enqueue("1", "", epoch.Add(5*time.Second), "")
				sched.Run(ctx, StopWhenDrained())
				assert.Loosely(t, clock.Now(ctx).Sub(epoch), should.Equal(10*time.Second))
				assert.Loosely(t, exec.tasks, should.HaveLength(2))
			})
		})

		t.Run("Run(StopAfterTask)", func(t *ftt.Test) {
			t.Run("Stops immediately after the right task if ran serially", func(t *ftt.Test) {
				enqueue("1", "", epoch.Add(3*time.Second), "classA")
				enqueue("2", "", epoch.Add(6*time.Second), "classB")
				enqueue("3", "", epoch.Add(9*time.Second), "classB")
				sched.Run(ctx, StopAfterTask("classB"))
				assert.Loosely(t, payloads(exec.tasks), should.Resemble([]string{"1", "2"}))

				t.Run("Doesn't take into account previously executed tasks", func(t *ftt.Test) {
					sched.Run(ctx, StopAfterTask("classB"))
					assert.Loosely(t, payloads(exec.tasks), should.Resemble([]string{"1", "2", "3"}))
				})
			})

			t.Run("Stops immediately after the right task in a chain if ran serially", func(t *ftt.Test) {
				exec.execute = func(payload string, _ *Task) bool {
					switch payload {
					case "1":
						enqueue("2", "", clock.Now(ctx).Add(5*time.Second), "classB")
					case "2":
						enqueue("3", "", clock.Now(ctx).Add(5*time.Second), "classB")
					}
					return true
				}
				enqueue("1", "", time.Time{}, "classA")
				sched.Run(ctx, StopAfterTask("classB"))
				assert.Loosely(t, payloads(exec.tasks), should.Resemble([]string{"1", "2"}))
			})

			t.Run("Stops eventually if ran in parallel", func(t *ftt.Test) {
				// Generate task tree:
				//             Z
				//       ZA         ZB
				//    ZAA  ZAB   ZBA  ZBB
				exec.execute = func(payload string, _ *Task) bool {
					if len(payload) <= 3 {
						enqueue(payload+"A", "", time.Time{}, "classA")
						enqueue(payload+"B", "", time.Time{}, "classB")
					}
					return true
				}
				enqueue("Z", "", time.Time{}, "classZ")

				sched.Run(ctx, StopAfterTask("classA"), ParallelExecute())
				// At least Z and at least one of ZA, ZAA, ZBA must have been executed.
				exec.waitForTasks(t, 2)
				exec.m.Lock()
				ps := payloads(exec.tasks)
				exec.m.Unlock()
				found := false
				for _, p := range ps {
					if strings.HasSuffix(p, "A") {
						found = true
					}
				}
				assert.Loosely(t, found, should.BeTrue)
			})
		})

		t.Run("Run(StopBeforeTask)", func(t *ftt.Test) {
			t.Run("Stops after the prior task if ran serially", func(t *ftt.Test) {
				enqueue("1", "", epoch.Add(2*time.Second), "classA")
				enqueue("2", "", epoch.Add(4*time.Second), "classB")
				enqueue("3", "", epoch.Add(6*time.Second), "classA")
				enqueue("4", "", epoch.Add(8*time.Second), "classB")
				sched.Run(ctx, StopBeforeTask("classB"))
				assert.Loosely(t, payloads(exec.tasks), should.Resemble([]string{"1"}))

				t.Run("Even if it doesn't run anything", func(t *ftt.Test) {
					sched.Run(ctx, StopBeforeTask("classB"))
					// The payloasd must be exactly same.
					assert.Loosely(t, payloads(exec.tasks), should.Resemble([]string{"1"}))
				})
			})

			t.Run("Takes into account newly scheduled tasks", func(t *ftt.Test) {
				exec.execute = func(payload string, _ *Task) bool {
					switch payload {
					case "1":
						enqueue("2->a", "", clock.Now(ctx).Add(2*time.Second), "classA")
						enqueue("2->b", "", clock.Now(ctx).Add(2*time.Second), "classA")
					case "2->a":
						enqueue("3a", "", clock.Now(ctx).Add(8*time.Second), "classA") // eta after 3b
					case "2->b":
						enqueue("3b", "", clock.Now(ctx).Add(6*time.Second), "classB") // eta before 3a
					}
					return true
				}
				enqueue("1", "", time.Time{}, "classA")

				t.Run("Stops before 3a and 3b if run serially", func(t *ftt.Test) {
					sched.Run(ctx, StopBeforeTask("classB"))
					assert.Loosely(t, payloads(exec.tasks), should.Resemble([]string{"1", "2->a", "2->b"}))
				})
				t.Run("Stops before 3b, but 3a may be executed, if run in parallel", func(t *ftt.Test) {
					sched.Run(ctx, StopBeforeTask("classB"), ParallelExecute())
					ps := orderByPayload(exec.tasks)
					assert.Loosely(t, ps[:3], should.Resemble([]string{"1", "2->a", "2->b"}))
					assert.Loosely(t, ps[3:], should.NotContain("3b"))
				})
			})
		})
	})
}

func TestTaskList(t *testing.T) {
	t.Parallel()

	ftt.Run("With task list", t, func(t *ftt.Test) {
		var epoch = time.Unix(1442540000, 0)

		task := func(payload int, exec bool, eta int, class, name string) *Task {
			return &Task{
				Name:      name,
				Class:     class,
				Executing: exec,
				ETA:       epoch.Add(time.Duration(eta) * time.Second),
				Payload:   &durationpb.Duration{Seconds: int64(payload)},
			}
		}

		tl := TaskList{
			task(0, true, 3, "", ""),
			task(1, false, 1, "", ""),
			task(2, true, 2, "", ""),
			task(3, false, 4, "", ""),
			task(4, true, 5, "classB", ""),
			task(5, true, 5, "classA", ""),
			task(6, true, 5, "classA", "b"),
			task(7, true, 5, "classA", "a"),
		}

		t.Run("Payloads", func(t *ftt.Test) {
			assert.Loosely(t, tl.Payloads(), should.Resemble([]protoreflect.ProtoMessage{
				&durationpb.Duration{Seconds: 0},
				&durationpb.Duration{Seconds: 1},
				&durationpb.Duration{Seconds: 2},
				&durationpb.Duration{Seconds: 3},
				&durationpb.Duration{Seconds: 4},
				&durationpb.Duration{Seconds: 5},
				&durationpb.Duration{Seconds: 6},
				&durationpb.Duration{Seconds: 7},
			}))
		})

		t.Run("Executing/Pending", func(t *ftt.Test) {
			assert.Loosely(t, tl.Executing().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
				&durationpb.Duration{Seconds: 0},
				&durationpb.Duration{Seconds: 2},
				&durationpb.Duration{Seconds: 4},
				&durationpb.Duration{Seconds: 5},
				&durationpb.Duration{Seconds: 6},
				&durationpb.Duration{Seconds: 7},
			}))

			assert.Loosely(t, tl.Pending().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
				&durationpb.Duration{Seconds: 1},
				&durationpb.Duration{Seconds: 3},
			}))
		})

		t.Run("SortByETA", func(t *ftt.Test) {
			assert.Loosely(t, tl.SortByETA().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
				&durationpb.Duration{Seconds: 2},
				&durationpb.Duration{Seconds: 0},
				&durationpb.Duration{Seconds: 5},
				&durationpb.Duration{Seconds: 7},
				&durationpb.Duration{Seconds: 6},
				&durationpb.Duration{Seconds: 4},
				&durationpb.Duration{Seconds: 1},
				&durationpb.Duration{Seconds: 3},
			}))
		})
	})

	ftt.Run("TasksCollector", t, func(t *ftt.Test) {
		var tl TaskList
		cb := TasksCollector(&tl)
		cb(context.Background(), &Task{})
		cb(context.Background(), &Task{})
		assert.Loosely(t, tl, should.HaveLength(2))
	})
}

type testExecutor struct {
	ctx     context.Context
	execute func(payload string, t *Task) bool
	ch      chan *Task

	m     sync.Mutex
	tasks []*Task
}

func (exe *testExecutor) Execute(ctx context.Context, t *Task, done func(retry bool)) {
	t = t.Copy()

	success := true
	if exe.execute != nil {
		success = exe.execute(t.Task.GetHttpRequest().Url, t)
	}

	exe.m.Lock()
	exe.tasks = append(exe.tasks, t)
	exe.m.Unlock()
	exe.ch <- t
	done(!success)
}

func (exe *testExecutor) waitForTasks(t testing.TB, n int) {
	t.Helper()

	for ; n > 0; n-- {
		select {
		case <-exe.ch:
		case <-exe.ctx.Done():
			assert.Loosely(t, "the scheduler is stuck", should.BeNil, truth.LineContext())
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
