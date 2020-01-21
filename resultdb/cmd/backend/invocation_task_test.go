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

package main

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLeaseInvocationTask(t *testing.T) {
	Convey(`leaseInvocationTask`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		now := clock.Now(ctx)

		invID := span.InvocationID("inv")
		invTask := &internalpb.InvocationTask{}

		test := func(processAfter time.Time, expectedProcessAfter time.Time, expectedShouldRunTask bool) {
			key := span.TaskKey{
				InvocationID: invID,
				TaskID:       "task_1",
			}
			testutil.MustApply(ctx,
				span.InsertInvocationTask(key, invTask, processAfter),
			)

			_, shouldRunTask, err := leaseInvocationTask(ctx, key)
			So(err, ShouldBeNil)
			So(shouldRunTask, ShouldEqual, expectedShouldRunTask)

			// Check the task's ProcessAfter is updated.
			var newProcessAfter time.Time
			txn := span.Client(ctx).Single()
			err = span.ReadRow(ctx, txn, "InvocationTasks", invID.Key("task_1"), map[string]interface{}{
				"ProcessAfter": &newProcessAfter,
			})
			So(err, ShouldBeNil)
			So(newProcessAfter, ShouldEqual, expectedProcessAfter)
		}

		Convey(`succeeded`, func() {
			test(now.Add(-time.Hour), now.Add(taskLeaseTime), true)
		})

		Convey(`skipped`, func() {
			test(now.Add(time.Hour), now.Add(time.Hour), false)
		})
	})

	Convey(`deleteInvocationTask`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		now := clock.Now(ctx)

		invID := span.InvocationID("inv")

		key := span.TaskKey{
			InvocationID: invID,
			TaskID:       "task_4",
		}
		testutil.MustApply(ctx,
			span.InsertInvocationTask(key, &internalpb.InvocationTask{}, now.Add(-time.Hour)),
		)

		err := deleteInvocationTask(ctx, key)
		So(err, ShouldBeNil)

		txn := span.Client(ctx).Single()
		var taskID string
		err = span.ReadRow(ctx, txn, "InvocationTasks", invID.Key("task_4"), map[string]interface{}{
			"TaskID": &taskID,
		})
		So(err, ShouldErrLike, "row not found")
	})
}
