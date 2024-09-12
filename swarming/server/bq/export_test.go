// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bq

import (
	"context"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/bq/taskspb"
)

func TestScheduleExportTasks(t *testing.T) {
	t.Parallel()

	// TODO(jonahhooper) Add custom error handler to test failure to schedule
	// a task.
	// See: https://crrev.com/c/5054492/11..14/swarming/server/bq/export.go#b97
	ftt.Run("With mocks", t, func(t *ftt.Test) {
		var testTime = testclock.TestRecentTimeUTC.Truncate(time.Microsecond)

		disp := &tq.Dispatcher{}
		registerTQTasks(disp, nil)

		ctx, _ := testclock.UseTime(context.Background(), testTime)
		ctx = memory.Use(ctx)
		ctx, tasks := tq.TestingContext(ctx, disp)

		t.Run("Creates 4 export tasks", func(t *ftt.Test) {
			start := testTime.Add(-minEventAge - time.Minute - time.Second)
			schedule := ExportSchedule{
				ID:         TaskRequests,
				NextExport: start,
			}
			err := datastore.Put(ctx, &schedule)
			assert.Loosely(t, err, should.BeNil)
			err = scheduleExportTasks(ctx, disp, "", maxTasksToSchedule, "foo", "bar", TaskRequests)
			assert.Loosely(t, err, should.BeNil)
			expected := []*taskspb.ExportInterval{
				{
					OperationId:  "task_requests:1454472125:15",
					CloudProject: "foo",
					Dataset:      "bar",
					TableName:    TaskRequests,
					Start:        timestamppb.New(start),
					Duration:     durationpb.New(exportDuration),
				},
				{
					OperationId:  "task_requests:1454472140:15",
					CloudProject: "foo",
					Dataset:      "bar",
					TableName:    TaskRequests,
					Start:        timestamppb.New(start.Add(1 * exportDuration)),
					Duration:     durationpb.New(exportDuration),
				},
				{
					OperationId:  "task_requests:1454472155:15",
					CloudProject: "foo",
					Dataset:      "bar",
					TableName:    TaskRequests,
					Start:        timestamppb.New(start.Add(2 * exportDuration)),
					Duration:     durationpb.New(exportDuration),
				},
				{
					OperationId:  "task_requests:1454472170:15",
					CloudProject: "foo",
					Dataset:      "bar",
					TableName:    TaskRequests,
					Start:        timestamppb.New(start.Add(3 * exportDuration)),
					Duration:     durationpb.New(exportDuration),
				},
			}
			assert.Loosely(t, tasks.Tasks(), should.HaveLength(len(expected)))
			payloads := make([]*taskspb.ExportInterval, len(expected))
			for idx, tsk := range tasks.Tasks().Payloads() {
				payloads[idx] = tsk.(*taskspb.ExportInterval)
			}
			// We don't care about order and the test scheduler doesn't appear
			// to return results in order all the time so sort results by time
			sort.Slice(payloads, func(i, j int) bool {
				return payloads[i].Start.AsTime().Before(payloads[j].Start.AsTime())
			})
			assert.Loosely(t, payloads, should.Resemble(expected))
			err = datastore.Get(ctx, &schedule)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, schedule.NextExport, should.Match(start.Add(4*exportDuration)))
		})

		t.Run("Creates 0 export tasks if ExportSchedule doesn't exist", func(t *ftt.Test) {
			err := scheduleExportTasks(ctx, disp, "", maxTasksToSchedule, "foo", "bar", TaskRequests)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tasks.Tasks(), should.BeEmpty)
			schedule := ExportSchedule{ID: TaskRequests}
			err = datastore.Get(ctx, &schedule)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, schedule.NextExport, should.Match(testTime.Add(-minEventAge).Truncate(time.Minute)))
		})

		t.Run("We cannot create more than 20 tasks at a time", func(t *ftt.Test) {
			start := testTime.Add(-(2 + 10) * time.Minute).Truncate(time.Minute)
			assert.Loosely(t, datastore.Put(ctx, &ExportSchedule{
				ID:         TaskRequests,
				NextExport: start,
			}), should.BeNil)
			err := scheduleExportTasks(ctx, disp, "", maxTasksToSchedule, "foo", "bar", TaskRequests)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tasks.Tasks(), should.HaveLength(20))
		})

		t.Run("We cannot schedule an exportTask in the future", func(t *ftt.Test) {
			start := testTime.Add(5 * time.Minute)
			assert.Loosely(t, datastore.Put(ctx, &ExportSchedule{
				ID:         TaskRequests,
				NextExport: start,
			}), should.BeNil)
			err := scheduleExportTasks(ctx, disp, "", maxTasksToSchedule, "foo", "bar", TaskRequests)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tasks.Tasks(), should.HaveLength(0))
		})

		t.Run("Custom key prefix and max tasks to schedule", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &ExportSchedule{
				ID:         "key-prefix:" + TaskRequests,
				NextExport: testTime.Add(-minEventAge - time.Minute),
			}), should.BeNil)
			err := scheduleExportTasks(ctx, disp, "key-prefix:", 1, "foo", "bar", TaskRequests)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tasks.Tasks(), should.HaveLength(1))
			task := tasks.Tasks().Payloads()[0].(*taskspb.ExportInterval)
			assert.Loosely(t, task.OperationId, should.Equal("key-prefix:task_requests:1454472126:15"))
		})
	})
}
