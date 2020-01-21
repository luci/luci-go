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

package spantest

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSampleInvocationTasks(t *testing.T) {
	Convey(`TestSampleInvocationTasks`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		now := clock.Now(ctx)
		task := &internalpb.InvocationTask{}
		testutil.MustApply(ctx,
			span.InsertInvocationTask("task1", "inv", task, now.Add(-time.Hour)),
			span.InsertInvocationTask("task2", "inv", task, now.Add(-time.Hour)),
			span.InsertInvocationTask("task3", "inv", task, now.Add(-time.Hour)),
			span.InsertInvocationTask("task4", "inv", task, now),
			span.InsertInvocationTask("task5", "inv", task, now.Add(time.Hour)),
		)

		rows, err := span.SampleInvocationTasks(ctx, now, 3)
		So(err, ShouldBeNil)
		So(rows, ShouldHaveLength, 3)
		So(rows, ShouldNotContain, "task5")
	})
}
