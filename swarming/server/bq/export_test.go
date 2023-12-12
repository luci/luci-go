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
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/swarming/server/bq/taskspb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	RegisterTQTasks()
}

func TestCreateExportTask(t *testing.T) {
	t.Parallel()

	// TODO(jonahhooper) Add custom error handler to test failure to schedule
	// a task.
	// See: https://crrev.com/c/5054492/11..14/swarming/server/bq/export.go#b97
	Convey("With mocks", t, func() {
		setup := func() (context.Context, testclock.TestClock, *tqtesting.Scheduler) {
			ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
			ctx = memory.Use(ctx)
			ctx, skdr := tq.TestingContext(ctx, nil)
			return ctx, tc, skdr
		}
		Convey("Creates 4 ExportTasks", func() {
			ctx, tc, skdr := setup()
			cutoff := tc.Now().UTC().Add(-latestAge)
			start := cutoff.Add(-time.Minute)
			// It appears that datastore testing implementation strips away
			// nanosecond time precision.
			// Truncate by microsecond matches this behaviour in UTs
			start = start.Truncate(time.Microsecond)
			state := ExportSchedule{
				Key:        exportScheduleKey(ctx, TaskRequests),
				NextExport: start,
			}
			err := datastore.Put(ctx, &state)
			So(err, ShouldBeNil)
			err = ScheduleExportTasks(ctx, "foo", "bar", TaskRequests)
			So(err, ShouldBeNil)
			expected := []*taskspb.CreateExportTask{
				{
					CloudProject: "foo",
					Dataset:      "bar",
					TableName:    TaskRequests,
					Start:        timestamppb.New(start),
					Duration:     durationpb.New(exportDuration),
				},
				{
					CloudProject: "foo",
					Dataset:      "bar",
					TableName:    TaskRequests,
					Start:        timestamppb.New(start.Add(1 * exportDuration)),
					Duration:     durationpb.New(exportDuration),
				},
				{
					CloudProject: "foo",
					Dataset:      "bar",
					TableName:    TaskRequests,
					Start:        timestamppb.New(start.Add(2 * exportDuration)),
					Duration:     durationpb.New(exportDuration),
				},
				{
					CloudProject: "foo",
					Dataset:      "bar",
					TableName:    TaskRequests,
					Start:        timestamppb.New(start.Add(3 * exportDuration)),
					Duration:     durationpb.New(exportDuration),
				},
			}
			So(skdr.Tasks(), ShouldHaveLength, len(expected))
			payloads := make([]*taskspb.CreateExportTask, len(expected))
			for idx, tsk := range skdr.Tasks().Payloads() {
				payloads[idx] = tsk.(*taskspb.CreateExportTask)
			}
			// We don't care about order and the test scheduler doesn't appear
			// to return results in order all the time so sort results by time
			sort.Slice(payloads, func(i, j int) bool {
				return payloads[i].Start.AsTime().Before(payloads[j].Start.AsTime())
			})
			So(payloads, ShouldResembleProto, expected)
			err = datastore.Get(ctx, &state)
			So(err, ShouldBeNil)
			So(state.NextExport, ShouldEqual, start.Add(4*exportDuration))
		})
		Convey("Creates 0 export tasks if ExportSchedule doesn't exist", func() {
			ctx, tc, skdr := setup()
			err := ScheduleExportTasks(ctx, "foo", "bar", TaskRequests)
			So(err, ShouldBeNil)
			So(skdr.Tasks(), ShouldBeEmpty)
			state := ExportSchedule{Key: exportScheduleKey(ctx, TaskRequests)}
			err = datastore.Get(ctx, &state)
			So(err, ShouldBeNil)
			So(state.NextExport, ShouldEqual, tc.Now().Truncate(time.Minute))
		})
		Convey("We cannot create more than 20 tasks at a time", func() {
			ctx, tc, skdr := setup()
			start := tc.Now().Add(-(2 + 10) * time.Minute).Truncate(time.Minute)
			state := ExportSchedule{
				Key:        exportScheduleKey(ctx, TaskRequests),
				NextExport: start,
			}
			So(datastore.Put(ctx, &state), ShouldBeNil)
			err := ScheduleExportTasks(ctx, "foo", "bar", TaskRequests)
			So(err, ShouldBeNil)
			So(skdr.Tasks(), ShouldHaveLength, 20)
		})
		Convey("We cannot schedule an exportTask in the future", func() {
			ctx, tc, skdr := setup()
			start := tc.Now().Add(5 * time.Minute)
			state := ExportSchedule{
				Key:        exportScheduleKey(ctx, TaskRequests),
				NextExport: start,
			}
			So(datastore.Put(ctx, &state), ShouldBeNil)
			err := ScheduleExportTasks(ctx, "foo", "bar", TaskRequests)
			So(err, ShouldBeNil)
			So(skdr.Tasks(), ShouldHaveLength, 0)
		})
	})
}
