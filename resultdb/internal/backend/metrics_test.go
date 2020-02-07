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

package backend

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetrics(t *testing.T) {
	Convey(`TestQueryOldestTaskAge`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		start := clock.Now(ctx).UTC()

		// Save the tasks to Spanner separately so they have different create time.
		// task1 is the oldest of all tasks, but it's not of the requested type.
		testutil.MustApply(ctx,
			tasks.Enqueue(tasks.TryFinalizeInvocation, "task1", "inv", "payload", start.Add(-2*time.Hour)),
		)
		// The oldest BQExport task should be task2.
		expectedCT := testutil.MustApply(ctx,
			tasks.Enqueue(tasks.BQExport, "task2", "inv", "payload", start.Add(-time.Hour)),
		)
		testutil.MustApply(ctx,
			tasks.Enqueue(tasks.BQExport, "task3", "inv", "payload", start.Add(time.Hour)),
		)

		Convey(`successfully get the oldest task`, func() {
			ct, err := queryOldestTask(ctx, tasks.BQExport)
			So(err, ShouldBeNil)
			So(ct.IsNull(), ShouldBeFalse)
			So(ct.Time, ShouldEqual, expectedCT)
		})

		Convey(`no task of a type`, func() {
			randomType := tasks.Type("random")
			ct, err := queryOldestTask(ctx, randomType)
			So(err, ShouldBeNil)
			So(ct.IsNull(), ShouldBeTrue)
		})
	})
}
