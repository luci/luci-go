// Copyright 2023 The LUCI Authors.
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

func TestNewestFirst(t *testing.T) {
	t.Parallel()

	Convey("With simulator", t, func(c C) {
		invocationDuration := time.Hour // may be modified in tests below.
		s := Simulator{
			OnRequest: func(s *Simulator, r task.Request) time.Duration {
				return invocationDuration
			},
			OnDebugLog: func(format string, args ...any) {
				_, _ = c.Printf(format+"\n", args...)
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

		Convey("Pending timeout must be positive", func() {
			s.Policy, err = NewestFirstPolicy(2, -time.Hour)
			So(err, ShouldNotBeNil)
		})

		Convey("Newest first works", func() {
			Convey("at least 1 trigger", func() {
				s.Policy, err = NewestFirstPolicy(1, 300*invocationDuration)
				So(err, ShouldBeNil)

				// Add exactly one trigger.
				addTriggers(noDelay, 1)
				So(s.Invocations, ShouldHaveLength, 1)
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t-001"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "data-001")
				So(s.PendingTriggers, ShouldHaveLength, 0)
				So(s.DiscardedTriggers, ShouldHaveLength, 0)
			})

			Convey("process newer triggers first", func() {
				s.Policy, err = NewestFirstPolicy(1, 300*invocationDuration)
				So(err, ShouldBeNil)

				const N = 3
				addTriggers(noDelay, N)
				for i := N - 1; i >= 0; i-- {
					So(s.Invocations, ShouldHaveLength, N-i)
					So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{fmt.Sprintf("t-%03d", i+1)})
					So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, fmt.Sprintf("data-%03d", i+1))
					So(s.PendingTriggers, ShouldHaveLength, i)
					So(s.DiscardedTriggers, ShouldHaveLength, 0)
					addTriggers(invocationDuration, 0)
				}
				So(s.Invocations, ShouldHaveLength, N)
				So(s.DiscardedTriggers, ShouldHaveLength, 0)
			})

			Convey("respects pending timeout", func() {
				const N = 2
				const extra = 2
				s.Policy, err = NewestFirstPolicy(1, N*invocationDuration)
				So(err, ShouldBeNil)

				// Add more extra triggers than we can fit running serially in the pending timeout.
				// This extra trigger will be discarded.
				addTriggers(noDelay, N+extra)

				So(s.Invocations, ShouldHaveLength, 1)
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t-004"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "data-004")
				So(s.PendingTriggers, ShouldHaveLength, 1+extra)
				So(s.DiscardedTriggers, ShouldHaveLength, 0)

				// Advance time, allowing an invocation to finish.
				addTriggers(invocationDuration, 0)

				So(s.Invocations, ShouldHaveLength, 2)
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t-003"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "data-003")
				So(s.PendingTriggers, ShouldHaveLength, extra)
				So(s.DiscardedTriggers, ShouldHaveLength, 0)

				// Advance time, allowing an invocation to finish.
				addTriggers(invocationDuration, 0)

				So(s.Invocations, ShouldHaveLength, N)
				So(s.DiscardedTriggers, ShouldHaveLength, extra)
			})

			Convey("pending timeout discards triggers due to starvation", func() {
				const N = 3
				s.Policy, err = NewestFirstPolicy(1, N*invocationDuration)
				So(err, ShouldBeNil)

				// Add more extra triggers than we can fit running serially in the pending timeout.
				// This extra trigger will be discarded.
				extra := 1
				addTriggers(noDelay, 1+extra)
				for i := 0; i < N; i++ {
					So(s.Invocations, ShouldHaveLength, i+1)
					So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{fmt.Sprintf("t-%03d", i+1+extra)})
					So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, fmt.Sprintf("data-%03d", i+1+extra))
					So(s.PendingTriggers, ShouldHaveLength, 1)
					So(s.DiscardedTriggers, ShouldHaveLength, 0)

					// Add the trigger first. We want a newer trigger to always come in before a slot frees up so
					// that the extra triggers get starved.
					addTriggers(invocationDuration/2, 1)
					addTriggers(invocationDuration/2, 0)
				}
				So(s.Invocations, ShouldHaveLength, N+1)
				So(s.DiscardedTriggers, ShouldHaveLength, extra)
			})

			Convey("very short timeout causes immediate discard", func() {
				s.Policy, err = NewestFirstPolicy(1, time.Nanosecond)
				So(err, ShouldBeNil)

				for i := 0; i < 3; i++ {
					addTriggers(invocationDuration, 2)
					So(s.Invocations, ShouldHaveLength, i+1)
					So(s.PendingTriggers, ShouldHaveLength, 1)
					So(s.DiscardedTriggers, ShouldHaveLength, i)
				}
			})

			Convey("multiple concurrent invocations", func() {
				const concurrentInvocations = 2
				s.Policy, err = NewestFirstPolicy(concurrentInvocations, 2*invocationDuration)
				So(err, ShouldBeNil)

				addTriggers(noDelay, 6)

				So(s.Invocations, ShouldHaveLength, 2)
				So(s.PendingTriggers, ShouldHaveLength, 4)
				So(s.DiscardedTriggers, ShouldHaveLength, 0)

				addTriggers(invocationDuration, 2)

				So(s.Invocations, ShouldHaveLength, 4)
				So(s.PendingTriggers, ShouldHaveLength, 4)
				So(s.DiscardedTriggers, ShouldHaveLength, 0)

				addTriggers(invocationDuration, 2)

				So(s.Invocations, ShouldHaveLength, 6)
				So(s.PendingTriggers, ShouldHaveLength, 2)
				So(s.DiscardedTriggers, ShouldHaveLength, 2)
			})
		})
	})
}
