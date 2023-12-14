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

func TestGreedyBatching(t *testing.T) {
	t.Parallel()

	Convey("With simulator", t, func(c C) {
		var err error
		s := Simulator{
			OnRequest: func(s *Simulator, r task.Request) time.Duration {
				return time.Minute
			},
			OnDebugLog: func(format string, args ...any) {
				c.Printf(format+"\n", args...)
			},
		}

		Convey("Max concurrent invocations must be positive", func() {
			s.Policy, err = GreedyBatchingPolicy(0, 1)
			So(err, ShouldNotBeNil)
		})

		Convey("Max batch size must be positive", func() {
			s.Policy, err = GreedyBatchingPolicy(1, 0)
			So(err, ShouldNotBeNil)
		})

		Convey("No invocations scheduled when reducer returns 0", func() {
			// A policy that always asks for 0 triggers to be combined.
			s.Policy, err = basePolicy(2, 1000, func(triggers []*internal.Trigger) int {
				return 0
			})
			So(err, ShouldBeNil)

			s.AddTrigger(0,
				internal.NoopTrigger("t1", "t1_data"),
				internal.NoopTrigger("t2", "t2_data"))
			So(s.PendingTriggers, ShouldHaveLength, 2)
			So(s.Invocations, ShouldHaveLength, 0)
			So(s.DiscardedTriggers, ShouldHaveLength, 0)
		})

		Convey("Unlimited batching works", func() {
			// A policy that allows two concurrent invocations with effectively
			// unlimited batch size.
			s.Policy, err = GreedyBatchingPolicy(2, 1000)
			So(err, ShouldBeNil)

			// Submit a bunch of triggers at once. They'll be all collapsed into one
			// invocation and properties of the last one will be used to derive its
			// parameters.
			s.AddTrigger(0,
				internal.NoopTrigger("t1", "t1_data"),
				internal.NoopTrigger("t2", "t2_data"),
				internal.NoopTrigger("t3", "t3_data"))

			// Yep, no pending triggers and only one invocation.
			So(s.PendingTriggers, ShouldHaveLength, 0)
			So(s.Invocations, ShouldHaveLength, 1)
			So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t1", "t2", "t3"})
			So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "t3_data")

			// Some time later, while the previous invocation is still running, submit
			// more triggers. They cause a new invocation immediately, since we are
			// allowed to have 2 concurrent invocations by the policy
			s.AddTrigger(30*time.Second,
				internal.NoopTrigger("t4", "t4_data"),
				internal.NoopTrigger("t5", "t5_data"))

			// Yep, no pending triggers and two invocations now.
			So(s.PendingTriggers, ShouldHaveLength, 0)
			So(s.Invocations, ShouldHaveLength, 2)
			So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t4", "t5"})

			// Some time later, while the previous two invocations are still running,
			// submit more triggers. They'll get queued up.
			s.AddTrigger(20*time.Second,
				internal.NoopTrigger("t6", "t6_data"),
				internal.NoopTrigger("t7", "t7_data"))

			// Yep. No new invocation.
			So(s.PendingTriggers, ShouldHaveLength, 2)
			So(s.Invocations, ShouldHaveLength, 2)

			// Wait until the first invocation finishes running. It will cause the
			// policy to collect all pending triggers and launch new invocation with
			// them.
			s.AdvanceTime(20 * time.Second)

			// Yep. No pending triggers anymore and have a new invocation running.
			So(s.Invocations[0].Running, ShouldBeFalse)
			So(s.PendingTriggers, ShouldHaveLength, 0)
			So(s.Invocations, ShouldHaveLength, 3)
			So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t6", "t7"})

			// When the second invocation finishes, nothing new happens.
			s.AdvanceTime(time.Minute)
			So(s.Invocations[1].Running, ShouldBeFalse)
			So(s.Invocations, ShouldHaveLength, 3)

			// Make sure the policy didn't discard anything.
			So(s.DiscardedTriggers, ShouldHaveLength, 0)
		})

		Convey("Limited batching works", func() {
			// A policy that allows two concurrent invocations with batch size 3.
			var err error
			s.Policy, err = GreedyBatchingPolicy(2, 3)
			So(err, ShouldBeNil)

			// Submit a bunch of triggers at once. They'll result in two invocations,
			// per batch size limits.
			s.AddTrigger(0,
				internal.NoopTrigger("t1", ""),
				internal.NoopTrigger("t2", ""),
				internal.NoopTrigger("t3", ""),
				internal.NoopTrigger("t4", ""))

			// Yep.
			So(s.PendingTriggers, ShouldHaveLength, 0)
			So(s.Invocations, ShouldHaveLength, 2)
			So(s.Invocations[0].Request.TriggerIDs(), ShouldResemble, []string{"t1", "t2", "t3"})
			So(s.Invocations[1].Request.TriggerIDs(), ShouldResemble, []string{"t4"})

			// Some time later while the invocations are still running, submit more
			// triggers.
			s.AddTrigger(30*time.Second,
				internal.NoopTrigger("t5", ""),
				internal.NoopTrigger("t6", ""),
				internal.NoopTrigger("t7", ""),
				internal.NoopTrigger("t8", ""))

			// Nothing happened.
			So(s.PendingTriggers, ShouldHaveLength, 4)
			So(s.Invocations, ShouldHaveLength, 2)

			// Wait until invocations finish. It will cause a triage and pending
			// triggers will be dispatched (unevenly again).
			s.AdvanceTime(40 * time.Second)

			// Yep.
			So(s.PendingTriggers, ShouldHaveLength, 0)
			So(s.Invocations, ShouldHaveLength, 4)
			So(s.Invocations[2].Request.TriggerIDs(), ShouldResemble, []string{"t5", "t6", "t7"})
			So(s.Invocations[3].Request.TriggerIDs(), ShouldResemble, []string{"t8"})

			// Make sure the policy didn't discard anything.
			So(s.DiscardedTriggers, ShouldHaveLength, 0)
		})
	})
}
