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
	"testing"
	"time"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogarithmicBatching(t *testing.T) {
	t.Parallel()

	Convey("With simulator", t, func(c C) {
		var err error
		s := Simulator{
			OnRequest: func(s *Simulator, r task.Request) time.Duration {
				return time.Minute
			},
			OnDebugLog: func(format string, args ...interface{}) {
				c.Printf(format+"\n", args...)
			},
		}

		Convey("Logarithm base must be at least 2", func() {
			s.Policy, err = LogarithmicBatchingPolicy(2, 1000, 1.0)
			So(err, ShouldNotBeNil)
		})

		Convey("Logarithmic batching works", func() {
			// A policy that allows 1 concurrent invocation with effectively unlimited
			// batch size and logarithm base of 2.
			s.Policy, err = LogarithmicBatchingPolicy(1, 1000, 2.0)
			So(err, ShouldBeNil)

			Convey("for 1 trigger", func() {
				// Add a single trigger. It should cause an invocation to be scheduled.
				s.AddTrigger(0, internal.NoopTrigger("t1", "t1_data"))

				// Yep, 0 pending triggers and one invocation with one trigger ID.
				So(s.PendingTriggers, ShouldHaveLength, 0)
				So(s.Invocations, ShouldHaveLength, 1)
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t1"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "t1_data")
			})

			Convey("for 2 triggers", func() {
				// Add 3 triggers at once, but only first one should be part of the
				// invocation (Log2(3) == 1).
				s.AddTrigger(0,
					internal.NoopTrigger("t2", "t2_data"),
					internal.NoopTrigger("t3", "t3_data"))

				// Yep, 2 pending triggers and one invocation.
				So(s.PendingTriggers, ShouldHaveLength, 1)
				So(s.Invocations, ShouldHaveLength, 1)
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t2"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "t2_data")
			})

			Convey("for 5 triggers", func() {
				// Add more triggers to cause two of them be collapsed into a single
				// invocation.
				s.AddTrigger(0,
					internal.NoopTrigger("t5", "t5_data"),
					internal.NoopTrigger("t6", "t6_data"),
					internal.NoopTrigger("t7", "t7_data"),
					internal.NoopTrigger("t8", "t8_data"),
					internal.NoopTrigger("t9", "t9_data"))

				// Yep, 2 pending triggers and 1 invocation.
				So(s.PendingTriggers, ShouldHaveLength, 3)
				So(s.Invocations, ShouldHaveLength, 1)
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t5", "t6"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "t6_data")
			})
		})
	})
}
