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
	Convey(`leaseInvocationTask succeeded`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		now := clock.Now(ctx)

		invID := span.InvocationID("inv")
		invTask := &internalpb.InvocationTask{}

		testutil.MustApply(ctx,
			span.InsertInvocationTask(invID, "task_1", invTask, now.Add(-time.Hour), true),
		)

		_, shouldRunTask, err := leaseInvocationTask(ctx, now, &span.TaskKey{
			InvocationID: invID,
			TaskID:       "task_1",
		})
		So(err, ShouldBeNil)
		So(shouldRunTask, ShouldEqual, true)

		// Check the task's ProcessAfter is updated.
		var processAfter time.Time
		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()
		err = span.ReadRow(ctx, txn, "InvocationTasks", invID.Key("task_1"), map[string]interface{}{
			"ProcessAfter": &processAfter,
		})
		So(err, ShouldBeNil)
		So(processAfter, ShouldEqual, now.Add(taskLeasingTime))
	})

	Convey(`leaseInvocationTask skipped`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		now := clock.Now(ctx)

		invID := span.InvocationID("inv")
		invTask := &internalpb.InvocationTask{}

		testutil.MustApply(ctx,
			span.InsertInvocationTask(invID, "task_1", invTask, now.Add(time.Hour), true),
		)

		_, shouldRunTask, err := leaseInvocationTask(ctx, now, &span.TaskKey{
			InvocationID: invID,
			TaskID:       "task_1",
		})
		So(err, ShouldBeNil)
		So(shouldRunTask, ShouldEqual, false)

		// Check the task's ProcessAfter is not updated.
		var processAfter time.Time
		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()
		err = span.ReadRow(ctx, txn, "InvocationTasks", invID.Key("task_1"), map[string]interface{}{
			"ProcessAfter": &processAfter,
		})
		So(err, ShouldBeNil)
		So(processAfter, ShouldEqual, now.Add(time.Hour))
	})
}

func TestDeleteInvocationTask(t *testing.T) {
	Convey(`deleteInvocationTask`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		now := clock.Now(ctx)

		invID := span.InvocationID("inv")

		testutil.MustApply(ctx,
			span.InsertInvocationTask(invID, "task_4", &internalpb.InvocationTask{}, now.Add(-time.Hour), true),
		)

		err := deleteInvocationTask(ctx, &span.TaskKey{
			InvocationID: invID,
			TaskID:       "task_4",
		})
		So(err, ShouldBeNil)

		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()
		var taskID string
		err = span.ReadRow(ctx, txn, "InvocationTasks", invID.Key("task_4"), map[string]interface{}{
			"TaskID": &taskID,
		})
		_ = taskID
		So(err, ShouldErrLike, "row not found")
	})
}
