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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/bq/taskspb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestScheduleExportTasks(t *testing.T) {
	t.Parallel()

	// TODO(jonahhooper) Add custom error handler to test failure to schedule
	// a task.
	// See: https://crrev.com/c/5054492/11..14/swarming/server/bq/export.go#b97
	Convey("With mocks", t, func() {
		var testTime = testclock.TestRecentTimeUTC.Truncate(time.Microsecond)

		disp := &tq.Dispatcher{}
		registerTQTasks(disp, nil)

		ctx, _ := testclock.UseTime(context.Background(), testTime)
		ctx = memory.Use(ctx)
		ctx, tasks := tq.TestingContext(ctx, disp)

		Convey("Creates 4 export tasks", func() {
			start := testTime.Add(-minEventAge - time.Minute - time.Second)
			schedule := ExportSchedule{
				ID:         TaskRequests,
				NextExport: start,
			}
			err := datastore.Put(ctx, &schedule)
			So(err, ShouldBeNil)
			err = scheduleExportTasks(ctx, disp, "", maxTasksToSchedule, "foo", "bar", TaskRequests)
			So(err, ShouldBeNil)
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
			So(tasks.Tasks(), ShouldHaveLength, len(expected))
			payloads := make([]*taskspb.ExportInterval, len(expected))
			for idx, tsk := range tasks.Tasks().Payloads() {
				payloads[idx] = tsk.(*taskspb.ExportInterval)
			}
			// We don't care about order and the test scheduler doesn't appear
			// to return results in order all the time so sort results by time
			sort.Slice(payloads, func(i, j int) bool {
				return payloads[i].Start.AsTime().Before(payloads[j].Start.AsTime())
			})
			So(payloads, ShouldResembleProto, expected)
			err = datastore.Get(ctx, &schedule)
			So(err, ShouldBeNil)
			So(schedule.NextExport, ShouldEqual, start.Add(4*exportDuration))
		})

		Convey("Creates 0 export tasks if ExportSchedule doesn't exist", func() {
			err := scheduleExportTasks(ctx, disp, "", maxTasksToSchedule, "foo", "bar", TaskRequests)
			So(err, ShouldBeNil)
			So(tasks.Tasks(), ShouldBeEmpty)
			schedule := ExportSchedule{ID: TaskRequests}
			err = datastore.Get(ctx, &schedule)
			So(err, ShouldBeNil)
			So(schedule.NextExport, ShouldEqual, testTime.Add(-minEventAge).Truncate(time.Minute))
		})

		Convey("We cannot create more than 20 tasks at a time", func() {
			start := testTime.Add(-(2 + 10) * time.Minute).Truncate(time.Minute)
			So(datastore.Put(ctx, &ExportSchedule{
				ID:         TaskRequests,
				NextExport: start,
			}), ShouldBeNil)
			err := scheduleExportTasks(ctx, disp, "", maxTasksToSchedule, "foo", "bar", TaskRequests)
			So(err, ShouldBeNil)
			So(tasks.Tasks(), ShouldHaveLength, 20)
		})

		Convey("We cannot schedule an exportTask in the future", func() {
			start := testTime.Add(5 * time.Minute)
			So(datastore.Put(ctx, &ExportSchedule{
				ID:         TaskRequests,
				NextExport: start,
			}), ShouldBeNil)
			err := scheduleExportTasks(ctx, disp, "", maxTasksToSchedule, "foo", "bar", TaskRequests)
			So(err, ShouldBeNil)
			So(tasks.Tasks(), ShouldHaveLength, 0)
		})

		Convey("Custom key prefix and max tasks to schedule", func() {
			So(datastore.Put(ctx, &ExportSchedule{
				ID:         "key-prefix:" + TaskRequests,
				NextExport: testTime.Add(-minEventAge - time.Minute),
			}), ShouldBeNil)
			err := scheduleExportTasks(ctx, disp, "key-prefix:", 1, "foo", "bar", TaskRequests)
			So(err, ShouldBeNil)
			So(tasks.Tasks(), ShouldHaveLength, 1)
			task := tasks.Tasks().Payloads()[0].(*taskspb.ExportInterval)
			So(task.OperationId, ShouldEqual, "key-prefix:task_requests:1454472126:15")
		})
	})
}

func TestMigrationState(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		var testTime = testclock.TestRecentTimeUTC.Truncate(time.Microsecond)
		var minAligned = testTime.Truncate(time.Minute)

		ctx, _ := testclock.UseTime(context.Background(), testTime)
		ctx = memory.Use(ctx)

		yes, _ := ShouldExport(ctx, "table_id", minAligned)
		So(yes, ShouldBeFalse)
		yes, _ = ShouldExport(ctx, "table_id", minAligned.Add(5*time.Minute))
		So(yes, ShouldBeFalse)

		So(SetMigrationState(ctx, "table_id", 5*time.Minute), ShouldBeNil)

		yes, _ = ShouldExport(ctx, "table_id", minAligned.Add(4*time.Minute+45*time.Second))
		So(yes, ShouldBeFalse)
		yes, _ = ShouldExport(ctx, "table_id", minAligned.Add(5*time.Minute))
		So(yes, ShouldBeTrue)
		yes, _ = ShouldExport(ctx, "table_id", minAligned.Add(5*time.Minute+15*time.Second))
		So(yes, ShouldBeTrue)
	})
}
