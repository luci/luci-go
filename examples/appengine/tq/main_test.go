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

package main

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/filter/txndefer"
	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQueue(t *testing.T) {
	t.Parallel()

	Convey("Chain works", t, func() {
		var epoch = time.Unix(1500000000, 0).UTC()

		// Need the test clock to emulate delayed tasks. Tick it whenever TQ waits.
		ctx, tc := testclock.UseTime(context.Background(), epoch)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if testclock.HasTags(t, tqtesting.ClockTag) {
				tc.Add(d)
			}
		})

		// Need the datastore fake with txndefer filter installed. This filter is
		// required when using server/tq with transactional tasks. AddTask calls
		// will panic otherwise. It is installed in production server contexts by
		// default.
		ctx = txndefer.FilterRDS(memory.Use(ctx))

		// A separate local dispatcher instance just for this test and a Cloud Tasks
		// scheduler fake that will actually schedule tasks.
		disp := tq.Dispatcher{}
		sched := disp.SchedulerForTest()

		// The object under test.
		chain := CountDownChain{}
		chain.Register(&disp)

		// Enqueue the first task, then simulate the Cloud Tasks run loop until
		// there's no more pending or executing tasks left.
		So(chain.Enqueue(ctx, 5), ShouldBeNil)
		sched.Run(ctx, tqtesting.StopWhenDrained())

		// Verify all expected entities have been created, and when expected.
		numbers := map[int64]time.Duration{}
		datastore.GetTestable(ctx).CatchupIndexes()
		datastore.Run(ctx, datastore.NewQuery("ExampleEntity"), func(e *ExampleEntity) {
			numbers[e.ID] = e.LastUpdate.Sub(epoch)
		})
		So(numbers, ShouldResemble, map[int64]time.Duration{
			5: 100 * time.Millisecond,
			4: 200 * time.Millisecond,
			3: 300 * time.Millisecond,
			2: 400 * time.Millisecond,
			1: 500 * time.Millisecond,
		})
	})
}
