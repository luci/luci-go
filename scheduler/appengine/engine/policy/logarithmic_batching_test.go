// Copyright 2018 The LUCI Authors.
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

package policy

import (
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogarithmicBatching(t *testing.T) {
	t.Parallel()

	Convey("With simulator", t, func(c C) {
		const invocationDuration = time.Minute
		s := Simulator{
			OnRequest: func(s *Simulator, r task.Request) time.Duration {
				return invocationDuration
			},
			OnDebugLog: func(format string, args ...interface{}) {
				c.Printf(format+"\n", args...)
			},
		}

		const noDelay = time.Duration(0)
		lastAddedTrigger := 0
		addTriggers := func(delay time.Duration, n int) {
			ts := make([]internal.Trigger, n)
			for i := range ts {
				lastAddedTrigger++
				ts[i] = internal.NoopTrigger(
					fmt.Sprintf("t-%03d", lastAddedTrigger),
					fmt.Sprintf("data-%03d", lastAddedTrigger),
				)
			}
			s.AddTrigger(delay, ts...)
		}

		var err error

		Convey("Logarithm base must be at least 1.0001", func() {
			s.Policy, err = LogarithmicBatchingPolicy(2, 1000, 1.0)
			So(err, ShouldNotBeNil)
		})

		Convey("Logarithmic batching works", func() {
			// A policy that allows 1 concurrent invocation with effectively unlimited
			// batch size and logarithm base of 2.
			const maxBatchSize = 5
			s.Policy, err = LogarithmicBatchingPolicy(1, maxBatchSize, 2.0)
			So(err, ShouldBeNil)

			Convey("at least 1 trigger", func() {
				// Add exactly one trigger; log(2,1) == 0.
				addTriggers(noDelay, 1)
				So(s.Invocations, ShouldHaveLength, 1)
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t-001"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "data-001")
				So(s.PendingTriggers, ShouldHaveLength, 0)
			})

			Convey("rounds down number of consumed triggers", func() {
				addTriggers(noDelay, 3)
				So(s.Invocations, ShouldHaveLength, 1)
				// log(2,3) = 1.584
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t-001"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "data-001")
				So(s.PendingTriggers, ShouldHaveLength, 2)
			})

			Convey("respects maxBatchSize", func() {
				N := 1 << (maxBatchSize + 2)
				addTriggers(noDelay, N)
				So(s.Invocations, ShouldHaveLength, 1)
				So(s.Last().Request.TriggerIDs(), ShouldHaveLength, maxBatchSize)
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, fmt.Sprintf("data-%03d", maxBatchSize))
				So(s.PendingTriggers, ShouldHaveLength, N-maxBatchSize)
			})

			Convey("Many triggers", func() {
				// Add 5 triggers.
				addTriggers(noDelay, 5)
				So(s.Invocations, ShouldHaveLength, 1)
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t-001", "t-002"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "data-002")
				So(s.PendingTriggers, ShouldHaveLength, 3)

				// Add a few triggers while the invocation is running.
				addTriggers(invocationDuration/4, 2)
				addTriggers(invocationDuration/4, 2)
				addTriggers(invocationDuration/4, 2)
				addTriggers(invocationDuration/4, 2)
				// Invocation is finsihed now, we have 11 = (3 old + 4*2 new triggers).
				So(s.Invocations, ShouldHaveLength, 2) // new invocation created.
				// log(2,11) = 3.459
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t-003", "t-004", "t-005"})
			})
		})
	})
}
