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
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/taskqueue"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"github.com/golang/protobuf/proto"
)

var epoch = time.Unix(1500000000, 0).UTC()

func TestTestable(t *testing.T) {
	t.Parallel()

	ftt.Run("With dispatcher", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(epoch))
		tqt := taskqueue.GetTestable(ctx)

		d := tq.Dispatcher{}
		tst := GetTestable(ctx, &d)

		var calls []proto.Message
		handler := func(ctx context.Context, payload proto.Message) error {
			calls = append(calls, payload)
			return nil
		}

		// Abuse some well-known proto type to simplify the test. It's doesn't
		// matter what proto type we use here as long as it is registered in
		// protobuf type registry.
		d.RegisterTask(&durationpb.Duration{}, handler, "q1", nil)
		d.RegisterTask(&emptypb.Empty{}, handler, "q2", nil)
		d.RegisterTask(&anypb.Any{}, handler, "q2", nil)

		t.Run("CreateQueues works", func(t *ftt.Test) {
			tst.CreateQueues()

			var queue []string
			for q := range tqt.GetScheduledTasks() {
				queue = append(queue, q)
			}
			sort.Strings(queue)
			assert.Loosely(t, queue, should.Resemble([]string{"default", "q1", "q2"}))
		})

		t.Run("GetScheduledTasks works", func(t *ftt.Test) {
			tst.CreateQueues()

			// Empty initially.
			assert.Loosely(t, tst.GetScheduledTasks(), should.BeNil)

			// Add a bunch.
			d.AddTask(ctx, &tq.Task{
				Payload: &durationpb.Duration{Seconds: 1},
				Delay:   30 * time.Second,
			})
			d.AddTask(ctx, &tq.Task{
				Payload: &durationpb.Duration{Seconds: 2},
				Delay:   10 * time.Second,
			})
			d.AddTask(ctx, &tq.Task{
				Payload: &durationpb.Duration{Seconds: 3},
			})
			d.AddTask(ctx, &tq.Task{
				Payload: &durationpb.Duration{Seconds: 4},
			})

			// Have them.
			tasks := tst.GetScheduledTasks()
			assert.Loosely(t, len(tasks), should.Equal(4))

			// Correct order. First to execute are in front.
			assert.Loosely(t, tasks[0].Payload, should.Resemble(&durationpb.Duration{Seconds: 3}))
			assert.Loosely(t, tasks[1].Payload, should.Resemble(&durationpb.Duration{Seconds: 4}))
			assert.Loosely(t, tasks[2].Payload, should.Resemble(&durationpb.Duration{Seconds: 2}))
			assert.Loosely(t, tasks[3].Payload, should.Resemble(&durationpb.Duration{Seconds: 1}))
		})

		t.Run("ExecuteTask works", func(t *ftt.Test) {
			tst.CreateQueues()

			d.AddTask(ctx, &tq.Task{Payload: &durationpb.Duration{Seconds: 1}})
			d.AddTask(ctx, &tq.Task{Payload: &emptypb.Empty{}})

			for _, task := range tst.GetScheduledTasks() {
				tst.ExecuteTask(ctx, task, nil)
			}
			assert.Loosely(t, calls, should.Resemble([]proto.Message{
				&emptypb.Empty{},
				&durationpb.Duration{Seconds: 1},
			}))
		})
	})
}

func TestRunSimulation(t *testing.T) {
	t.Parallel()

	ftt.Run("Setup", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(epoch))

		d := tq.Dispatcher{}
		tst := GetTestable(ctx, &d)

		addTask := func(ctx context.Context, idx int64, delay time.Duration, dedup string) {
			d.AddTask(ctx, &tq.Task{
				Payload:          &wrapperspb.Int64Value{Value: idx},
				ETA:              clock.Now(ctx).Add(delay),
				DeduplicationKey: dedup,
			})
		}

		toIndex := func(t Task) int64 {
			return t.Payload.(*wrapperspb.Int64Value).Value
		}

		toIndexes := func(arr TaskList) (out []int64) {
			// Hit Payloads() to generate code coverage.
			for _, p := range arr.Payloads() {
				out = append(out, p.(*wrapperspb.Int64Value).Value)
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
		handler := func(ctx context.Context, payload proto.Message) error {
			switch payload.(*wrapperspb.Int64Value).Value {
			case int64(errorOnIdx):
				return fmt.Errorf("task failure")
			case 1:
				addTask(ctx, 2, time.Second, "")
			case 2:
				addTask(ctx, 3, time.Second, "")
				addTask(ctx, 4, time.Second, "")
			case 3, 4:
				addTask(ctx, 5, 0, "dedup")
			case 5:
				addTask(ctx, 6, time.Second, "")
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
		d.RegisterTask(&wrapperspb.Int64Value{}, handler, "", nil)

		t.Run("Happy path", func(t *ftt.Test) {
			addTask(ctx, 1, 0, "")

			executed, pending, err := tst.RunSimulation(ctx, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(pending), should.BeZero)

			// Task executed in correct sequence and duplicated task is skipped.
			assert.Loosely(t, toIndexes(executed), should.Resemble([]int64{1, 2, 3, 4, 5, 6}))
			// The clock matches last task.
			assert.Loosely(t, clock.Now(ctx).Sub(epoch), should.Equal(3*time.Second))
		})

		t.Run("Deadline", func(t *ftt.Test) {
			addTask(ctx, 1, 0, "")

			executed, pending, err := tst.RunSimulation(ctx, &SimulationParams{
				Deadline: epoch.Add(2500 * time.Millisecond),
			})
			assert.Loosely(t, err, should.BeNil)

			// The last task is still pending.
			assert.Loosely(t, toIndexes(pending), should.Resemble([]int64{6}))
			// Tasks executed in correct sequence and duplicated task is skipped.
			assert.Loosely(t, toIndexes(executed), should.Resemble([]int64{1, 2, 3, 4, 5}))
			// The clock matches last executed task.
			assert.Loosely(t, clock.Now(ctx).Sub(epoch), should.Equal(2*time.Second))
		})

		t.Run("ShouldStopBefore", func(t *ftt.Test) {
			addTask(ctx, 1, 0, "")

			executed, pending, err := tst.RunSimulation(ctx, &SimulationParams{
				ShouldStopBefore: func(t Task) bool {
					return toIndex(t) == 5
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// The task we stopped before is still pending.
			assert.Loosely(t, toIndexes(pending), should.Resemble([]int64{5}))
			// Tasks executed in correct sequence.
			assert.Loosely(t, toIndexes(executed), should.Resemble([]int64{1, 2, 3, 4}))
			// The clock matches last executed task.
			assert.Loosely(t, clock.Now(ctx).Sub(epoch), should.Equal(2*time.Second))
		})

		t.Run("ShouldStopAfter", func(t *ftt.Test) {
			addTask(ctx, 1, 0, "")

			executed, pending, err := tst.RunSimulation(ctx, &SimulationParams{
				ShouldStopAfter: func(t Task) bool {
					return toIndex(t) == 5
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// The next task is the one submitted by the task we stopped at.
			assert.Loosely(t, toIndexes(pending), should.Resemble([]int64{6}))
			// Tasks executed in correct sequence.
			assert.Loosely(t, toIndexes(executed), should.Resemble([]int64{1, 2, 3, 4, 5}))
			// The clock matches last executed task.
			assert.Loosely(t, clock.Now(ctx).Sub(epoch), should.Equal(2*time.Second))
		})

		t.Run("Error", func(t *ftt.Test) {
			addTask(ctx, 1, 0, "")

			errorOnIdx = 3
			executed, pending, err := tst.RunSimulation(ctx, nil)
			assert.Loosely(t, err.Error(), should.Equal("task failure"))

			// Task 4 is still pending, since 3 is lexicographically earlier.
			assert.Loosely(t, toIndexes(pending), should.Resemble([]int64{4}))
			// Last one errored.
			assert.Loosely(t, toIndexes(executed), should.Resemble([]int64{1, 2, 3}))
			// The clock matches last executed task.
			assert.Loosely(t, clock.Now(ctx).Sub(epoch), should.Equal(2*time.Second))
		})

		t.Run("Unrecognized task", func(t *ftt.Test) {
			unknownTask := taskqueue.Task{
				Path:    "/unknown",
				Payload: []byte{1, 2, 3},
			}
			assert.Loosely(t, taskqueue.Add(ctx, "", &unknownTask), should.BeNil)

			addTask(ctx, 1, 0, "") // as in "Happy path"

			t.Run("Without UnknownTaskHandler", func(t *ftt.Test) {
				executed, pending, err := tst.RunSimulation(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("unrecognized TQ task for handler at /unknown"))
				assert.Loosely(t, len(executed), should.Equal(1)) // executed the bad task
				assert.Loosely(t, len(pending), should.Equal(1))  // the good recognized task is pending
			})

			t.Run("With UnknownTaskHandler", func(t *ftt.Test) {
				var unknown []*taskqueue.Task
				executed, pending, err := tst.RunSimulation(ctx, &SimulationParams{
					UnknownTaskHandler: func(t *taskqueue.Task) error {
						unknown = append(unknown, t)
						return nil
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(executed), should.Equal(7)) // executed all tasks + 1 bad, see "happy path"
				assert.Loosely(t, len(pending), should.BeZero)
				assert.Loosely(t, unknown, should.Resemble([]*taskqueue.Task{&unknownTask}))
			})
		})
	})
}
