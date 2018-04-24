// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"sort"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/taskqueue"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var epoch = time.Unix(1500000000, 0).UTC()

func TestTestable(t *testing.T) {
	t.Parallel()

	Convey("With dispatcher", t, func() {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(epoch))
		tqt := taskqueue.GetTestable(ctx)

		d := tq.Dispatcher{}
		tst := GetTestable(ctx, &d)

		var calls []proto.Message
		handler := func(c context.Context, payload proto.Message) error {
			calls = append(calls, payload)
			return nil
		}

		// Abuse some well-known proto type to simplify the test. It's doesn't
		// matter what proto type we use here as long as it is registered in
		// protobuf type registry.
		d.RegisterTask(&duration.Duration{}, handler, "q1", nil)
		d.RegisterTask(&empty.Empty{}, handler, "q2", nil)
		d.RegisterTask(&any.Any{}, handler, "q2", nil)

		Convey("CreateQueues works", func() {
			tst.CreateQueues()

			var queue []string
			for q := range tqt.GetScheduledTasks() {
				queue = append(queue, q)
			}
			sort.Strings(queue)
			So(queue, ShouldResemble, []string{"default", "q1", "q2"})
		})

		Convey("GetScheduledTasks works", func() {
			tst.CreateQueues()

			// Empty initially.
			So(tst.GetScheduledTasks(), ShouldBeNil)

			// Add a bunch.
			d.AddTask(ctx, &tq.Task{
				Payload: &duration.Duration{Seconds: 1},
				Delay:   30 * time.Second,
			})
			d.AddTask(ctx, &tq.Task{
				Payload: &duration.Duration{Seconds: 2},
				Delay:   10 * time.Second,
			})
			d.AddTask(ctx, &tq.Task{
				Payload: &duration.Duration{Seconds: 3},
			})
			d.AddTask(ctx, &tq.Task{
				Payload: &duration.Duration{Seconds: 4},
			})

			// Have them.
			tasks := tst.GetScheduledTasks()
			So(len(tasks), ShouldEqual, 4)

			// Correct order. First to execute are in front.
			So(tasks[0].Payload, ShouldResemble, &duration.Duration{Seconds: 3})
			So(tasks[1].Payload, ShouldResemble, &duration.Duration{Seconds: 4})
			So(tasks[2].Payload, ShouldResemble, &duration.Duration{Seconds: 2})
			So(tasks[3].Payload, ShouldResemble, &duration.Duration{Seconds: 1})
		})

		Convey("ExecuteTask works", func() {
			tst.CreateQueues()

			d.AddTask(ctx, &tq.Task{Payload: &duration.Duration{Seconds: 1}})
			d.AddTask(ctx, &tq.Task{Payload: &empty.Empty{}})

			for _, task := range tst.GetScheduledTasks() {
				tst.ExecuteTask(ctx, task, nil)
			}
			So(calls, ShouldResemble, []proto.Message{
				&empty.Empty{},
				&duration.Duration{Seconds: 1},
			})
		})
	})
}

func TestRunSimulation(t *testing.T) {
	t.Parallel()

	Convey("Setup", t, func() {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(epoch))

		d := tq.Dispatcher{}
		tst := GetTestable(ctx, &d)

		addTask := func(c context.Context, idx int64, delay time.Duration, dedup string) {
			d.AddTask(c, &tq.Task{
				Payload:          &wrappers.Int64Value{Value: idx},
				ETA:              clock.Now(c).Add(delay),
				DeduplicationKey: dedup,
			})
		}

		toIndex := func(t Task) int64 {
			return t.Payload.(*wrappers.Int64Value).Value
		}

		toIndexes := func(arr TaskList) (out []int64) {
			// Hit Payloads() to generate code coverage.
			for _, p := range arr.Payloads() {
				out = append(out, p.(*wrappers.Int64Value).Value)
			}
			return
		}

		// |---> ETA time grows to the right.
		// 1 ------> 2 -----> 3
		//            \       |
		//             \      v
		//              \     5  --------> 6
		//               \    ^
		//                \   |
		//                 -> 4
		errorOnIdx := -1
		handler := func(c context.Context, payload proto.Message) error {
			switch payload.(*wrappers.Int64Value).Value {
			case int64(errorOnIdx):
				return fmt.Errorf("task failure")
			case 1:
				addTask(c, 2, time.Second, "")
			case 2:
				addTask(c, 3, time.Second, "")
				addTask(c, 4, time.Second, "")
			case 3, 4:
				addTask(c, 5, 0, "dedup")
			case 5:
				addTask(c, 6, time.Second, "")
			case 6:
				return nil
			default:
				return fmt.Errorf("unknown task %s", payload)
			}
			return nil
		}

		// Abuse some well-known proto type to simplify the test. It's doesn't
		// matter what proto type we use here as long as it is registered in
		// protobuf type registry.
		d.RegisterTask(&wrappers.Int64Value{}, handler, "", nil)

		Convey("Happy path", func() {
			addTask(ctx, 1, 0, "")

			executed, pending, err := tst.RunSimulation(ctx, nil)
			So(err, ShouldBeNil)
			So(len(pending), ShouldEqual, 0)

			// Task executed in correct sequence and duplicated task is skipped.
			So(toIndexes(executed), ShouldResemble, []int64{1, 2, 3, 4, 5, 6})
			// The clock matches last task.
			So(clock.Now(ctx).Sub(epoch), ShouldEqual, 3*time.Second)
		})

		Convey("Deadline", func() {
			addTask(ctx, 1, 0, "")

			executed, pending, err := tst.RunSimulation(ctx, &SimulationParams{
				Deadline: epoch.Add(2500 * time.Millisecond),
			})
			So(err, ShouldBeNil)

			// The last task is still pending.
			So(toIndexes(pending), ShouldResemble, []int64{6})
			// Tasks executed in correct sequence and duplicated task is skipped.
			So(toIndexes(executed), ShouldResemble, []int64{1, 2, 3, 4, 5})
			// The clock matches last executed task.
			So(clock.Now(ctx).Sub(epoch), ShouldEqual, 2*time.Second)
		})

		Convey("ShouldStopBefore", func() {
			addTask(ctx, 1, 0, "")

			executed, pending, err := tst.RunSimulation(ctx, &SimulationParams{
				ShouldStopBefore: func(t Task) bool {
					return toIndex(t) == 5
				},
			})
			So(err, ShouldBeNil)

			// The task we stopped before is still pending.
			So(toIndexes(pending), ShouldResemble, []int64{5})
			// Tasks executed in correct sequence.
			So(toIndexes(executed), ShouldResemble, []int64{1, 2, 3, 4})
			// The clock matches last executed task.
			So(clock.Now(ctx).Sub(epoch), ShouldEqual, 2*time.Second)
		})

		Convey("ShouldStopAfter", func() {
			addTask(ctx, 1, 0, "")

			executed, pending, err := tst.RunSimulation(ctx, &SimulationParams{
				ShouldStopAfter: func(t Task) bool {
					return toIndex(t) == 5
				},
			})
			So(err, ShouldBeNil)

			// The next task is the one submitted by the task we stopped at.
			So(toIndexes(pending), ShouldResemble, []int64{6})
			// Tasks executed in correct sequence.
			So(toIndexes(executed), ShouldResemble, []int64{1, 2, 3, 4, 5})
			// The clock matches last executed task.
			So(clock.Now(ctx).Sub(epoch), ShouldEqual, 2*time.Second)
		})

		Convey("Error", func() {
			addTask(ctx, 1, 0, "")

			errorOnIdx = 3
			executed, pending, err := tst.RunSimulation(ctx, nil)
			So(err.Error(), ShouldEqual, "task failure")

			// Task 4 is still pending, since 3 is lexicographically earlier.
			So(toIndexes(pending), ShouldResemble, []int64{4})
			// Last one errored.
			So(toIndexes(executed), ShouldResemble, []int64{1, 2, 3})
			// The clock matches last executed task.
			So(clock.Now(ctx).Sub(epoch), ShouldEqual, 2*time.Second)
		})

		Convey("Unrecognized task", func() {
			unknownTask := taskqueue.Task{
				Path:    "/unknown",
				Payload: []byte{1, 2, 3},
			}
			So(taskqueue.Add(ctx, "", &unknownTask), ShouldBeNil)

			addTask(ctx, 1, 0, "") // as in "Happy path"

			Convey("Without UnknownTaskHandler", func() {
				executed, pending, err := tst.RunSimulation(ctx, nil)
				So(err, ShouldErrLike, "unrecognized TQ task for handler at /unknown")
				So(len(executed), ShouldEqual, 1) // executed the bad task
				So(len(pending), ShouldEqual, 1)  // the good recognized task is pending
			})

			Convey("With UnknownTaskHandler", func() {
				var unknown []*taskqueue.Task
				executed, pending, err := tst.RunSimulation(ctx, &SimulationParams{
					UnknownTaskHandler: func(t *taskqueue.Task) error {
						unknown = append(unknown, t)
						return nil
					},
				})
				So(err, ShouldBeNil)
				So(len(executed), ShouldEqual, 7) // executed all tasks + 1 bad, see "happy path"
				So(len(pending), ShouldEqual, 0)
				So(unknown, ShouldResemble, []*taskqueue.Task{&unknownTask})
			})
		})
	})
}
