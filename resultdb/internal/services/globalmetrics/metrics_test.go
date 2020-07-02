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

package globalmetrics

import (
	"testing"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUpdateTaskStats(t *testing.T) {
	Convey(`updateTaskStats`, t, func() {
		ctx, _ := tsmon.WithDummyInMemory(testutil.SpannerTestContext(t))
		now := clock.Now(ctx).UTC()

		testutil.MustApply(ctx,
			tasks.Enqueue(tasks.BQExport, "task1", "inv", "payload", now),
			tasks.Enqueue(tasks.BQExport, "task2", "inv", "payload", now),
			tasks.Enqueue(tasks.TryFinalizeInvocation, "task3", "inv", "payload", now),
			tasks.Enqueue(tasks.TryFinalizeInvocation, "task4", "inv", "payload", now),
			tasks.Enqueue(tasks.TryFinalizeInvocation, "task5", "inv", "payload", now),
		)

		Convey(`reports count metric`, func() {
			So(updateTaskStats(ctx), ShouldBeNil)
			So(taskCountMetric.Get(ctx, string(tasks.BQExport)), ShouldEqual, 2)
			So(taskCountMetric.Get(ctx, string(tasks.TryFinalizeInvocation)), ShouldEqual, 3)

			Convey(`with 0 if there is no task`, func() {
				tasks.Delete(ctx, tasks.BQExport, "task1")
				tasks.Delete(ctx, tasks.BQExport, "task2")

				So(updateTaskStats(ctx), ShouldBeNil)
				So(taskCountMetric.Get(ctx, string(tasks.BQExport)), ShouldEqual, 0)
				So(taskCountMetric.Get(ctx, string(tasks.TryFinalizeInvocation)), ShouldEqual, 3)
			})
		})

		Convey("reports oldest-task-age metric", func() {
			So(updateTaskStats(ctx), ShouldBeNil)
			So(oldestTaskMetric.Get(ctx, string(tasks.BQExport)), ShouldEqual, now.Unix())
			So(oldestTaskMetric.Get(ctx, string(tasks.TryFinalizeInvocation)), ShouldEqual, now.Unix())
		})

		Convey("stops reporting oldest-task-age, if task-count == 0", func() {
			So(updateTaskStats(ctx), ShouldBeNil)
			So(oldestTaskMetric.Get(ctx, string(tasks.BQExport)), ShouldEqual, now.Unix())
			So(oldestTaskMetric.Get(ctx, string(tasks.TryFinalizeInvocation)), ShouldEqual, now.Unix())

			tasks.Delete(ctx, tasks.TryFinalizeInvocation, "task3")
			tasks.Delete(ctx, tasks.TryFinalizeInvocation, "task4")
			tasks.Delete(ctx, tasks.TryFinalizeInvocation, "task5")

			So(updateTaskStats(ctx), ShouldBeNil)
			So(oldestTaskMetric.Get(ctx, string(tasks.BQExport)), ShouldEqual, now.Unix())
			// metric.Get() returns the zero value of the metric value type, if there is
			// no matching cell for the fields. Thus, tsmon.Store.Get() should be used
			// to find whether the cell no longer exists or not.
			fvs := []interface{}{string(tasks.TryFinalizeInvocation)}
			So(tsmon.Store(ctx).Get(ctx, oldestTaskMetric, now, fvs), ShouldBeNil)
		})
	})
}
