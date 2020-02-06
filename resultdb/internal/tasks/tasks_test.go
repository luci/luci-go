// Copyright 2019 The LUCI Authors.
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

package tasks

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTasks(t *testing.T) {
	Convey(`TestTasks`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		start := clock.Now(ctx).UTC()

		Convey(`Sample`, func() {
			testutil.MustApply(ctx,
				Enqueue(BQExport, "task1", "inv", "payload", start.Add(-time.Hour)),
				Enqueue(BQExport, "task2", "inv", "payload", start.Add(-time.Hour)),
				Enqueue(BQExport, "task3", "inv", "payload", start.Add(-time.Hour)),
				Enqueue(BQExport, "task4", "inv", "payload", start),
				Enqueue(BQExport, "task5", "inv", "payload", start.Add(time.Hour)),
			)

			rows, err := Sample(ctx, BQExport, start, 3)
			So(err, ShouldBeNil)
			So(rows, ShouldHaveLength, 3)
			So(rows, ShouldNotContain, "task5")
		})

		Convey(`Lease`, func() {
			test := func(processAfter time.Time, expectedProcessAfter time.Time, expectedLeaseErr error) {
				testutil.MustApply(ctx,
					Enqueue(BQExport, "task", "inv", []byte("payload"), processAfter),
				)

				invID, payload, err := Lease(ctx, BQExport, "task", time.Second)
				So(err, ShouldEqual, expectedLeaseErr)
				So(invID, ShouldEqual, span.InvocationID("inv"))
				So(payload, ShouldResemble, []byte("payload"))

				// Check the task's ProcessAfter is updated.
				var newProcessAfter time.Time
				txn := span.Client(ctx).Single()
				err = span.ReadRow(ctx, txn, "InvocationTasks", BQExport.Key("task"), map[string]interface{}{
					"ProcessAfter": &newProcessAfter,
				})
				So(err, ShouldBeNil)
				So(newProcessAfter, ShouldHappenWithin, time.Second, expectedProcessAfter)
			}

			Convey(`succeeded`, func() {
				test(start.Add(-time.Hour), start.Add(time.Second), nil)
			})

			Convey(`skipped`, func() {
				test(start.Add(time.Hour), start.Add(time.Hour), ErrConflict)
			})

			Convey(`Non-existing task`, func() {
				_, _, err := Lease(ctx, BQExport, "task", time.Second)
				So(err, ShouldEqual, ErrConflict)
			})
		})

		Convey(`Delete`, func() {
			testutil.MustApply(ctx,
				Enqueue(BQExport, "task", "inv", "payload", start.Add(-time.Hour)),
			)

			err := Delete(ctx, BQExport, "task")
			So(err, ShouldBeNil)

			txn := span.Client(ctx).Single()
			var taskID string
			err = span.ReadRow(ctx, txn, "InvocationTasks", BQExport.Key("task"), map[string]interface{}{
				"TaskId": &taskID,
			})
			So(err, ShouldErrLike, "row not found")
		})

		Convey(`QueryOldestTaskAge`, func() {
			// Save the tasks to Spanner separately so they have different create time.
			testutil.MustApply(ctx,
				Enqueue(TryFinalizeInvocation, "task1", "inv", "payload", start.Add(-2*time.Hour)),
			)
			testutil.MustApply(ctx,
				Enqueue(BQExport, "task2", "inv", "payload", start.Add(-time.Hour)),
			)
			testutil.MustApply(ctx,
				Enqueue(BQExport, "task3", "inv", "payload", start.Add(time.Hour)),
			)

			ct, err := QueryOldestTaskAge(ctx, BQExport)
			So(err, ShouldBeNil)

			// The oldest BQExport task age should be task2's age.
			var expectedCT time.Time
			err = span.ReadRow(ctx, span.Client(ctx).Single(), "InvocationTasks", BQExport.Key("task2"), map[string]interface{}{
				"CreateTime": &expectedCT,
			})
			So(err, ShouldBeNil)
			So(ct, ShouldEqual, expectedCT)
		})
	})
}
