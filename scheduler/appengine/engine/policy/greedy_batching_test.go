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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

func TestGreedyBatching(t *testing.T) {
	t.Parallel()

	ftt.Run("With simulator", t, func(c *ftt.Test) {
		var err error
		s := Simulator{
			OnRequest: func(s *Simulator, r task.Request) time.Duration {
				return time.Minute
			},
			OnDebugLog: func(format string, args ...any) {
				c.Logf(format+"\n", args...)
			},
		}

		c.Run("Max concurrent invocations must be positive", func(c *ftt.Test) {
			s.Policy, err = GreedyBatchingPolicy(0, 1)
			assert.Loosely(c, err, should.NotBeNil)
		})

		c.Run("Max batch size must be positive", func(c *ftt.Test) {
			s.Policy, err = GreedyBatchingPolicy(1, 0)
			assert.Loosely(c, err, should.NotBeNil)
		})

		c.Run("No invocations scheduled when reducer returns 0", func(c *ftt.Test) {
			// A policy that always asks for 0 triggers to be combined.
			s.Policy, err = basePolicy(2, 1000, func(triggers []*internal.Trigger) int {
				return 0
			})
			assert.Loosely(c, err, should.BeNil)

			s.AddTrigger(0,
				internal.NoopTrigger("t1", "t1_data"),
				internal.NoopTrigger("t2", "t2_data"))
			assert.Loosely(c, s.PendingTriggers, should.HaveLength(2))
			assert.Loosely(c, s.Invocations, should.HaveLength(0))
			assert.Loosely(c, s.DiscardedTriggers, should.HaveLength(0))
		})

		c.Run("Unlimited batching works", func(c *ftt.Test) {
			// A policy that allows two concurrent invocations with effectively
			// unlimited batch size.
			s.Policy, err = GreedyBatchingPolicy(2, 1000)
			assert.Loosely(c, err, should.BeNil)

			// Submit a bunch of triggers at once. They'll be all collapsed into one
			// invocation and properties of the last one will be used to derive its
			// parameters.
			s.AddTrigger(0,
				internal.NoopTrigger("t1", "t1_data"),
				internal.NoopTrigger("t2", "t2_data"),
				internal.NoopTrigger("t3", "t3_data"))

			// Yep, no pending triggers and only one invocation.
			assert.Loosely(c, s.PendingTriggers, should.HaveLength(0))
			assert.Loosely(c, s.Invocations, should.HaveLength(1))
			assert.Loosely(c, s.Last().Request.TriggerIDs(), should.Match([]string{"t1", "t2", "t3"}))
			assert.Loosely(c, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal("t3_data"))

			// Some time later, while the previous invocation is still running, submit
			// more triggers. They cause a new invocation immediately, since we are
			// allowed to have 2 concurrent invocations by the policy
			s.AddTrigger(30*time.Second,
				internal.NoopTrigger("t4", "t4_data"),
				internal.NoopTrigger("t5", "t5_data"))

			// Yep, no pending triggers and two invocations now.
			assert.Loosely(c, s.PendingTriggers, should.HaveLength(0))
			assert.Loosely(c, s.Invocations, should.HaveLength(2))
			assert.Loosely(c, s.Last().Request.TriggerIDs(), should.Match([]string{"t4", "t5"}))

			// Some time later, while the previous two invocations are still running,
			// submit more triggers. They'll get queued up.
			s.AddTrigger(20*time.Second,
				internal.NoopTrigger("t6", "t6_data"),
				internal.NoopTrigger("t7", "t7_data"))

			// Yep. No new invocation.
			assert.Loosely(c, s.PendingTriggers, should.HaveLength(2))
			assert.Loosely(c, s.Invocations, should.HaveLength(2))

			// Wait until the first invocation finishes running. It will cause the
			// policy to collect all pending triggers and launch new invocation with
			// them.
			s.AdvanceTime(20 * time.Second)

			// Yep. No pending triggers anymore and have a new invocation running.
			assert.Loosely(c, s.Invocations[0].Running, should.BeFalse)
			assert.Loosely(c, s.PendingTriggers, should.HaveLength(0))
			assert.Loosely(c, s.Invocations, should.HaveLength(3))
			assert.Loosely(c, s.Last().Request.TriggerIDs(), should.Match([]string{"t6", "t7"}))

			// When the second invocation finishes, nothing new happens.
			s.AdvanceTime(time.Minute)
			assert.Loosely(c, s.Invocations[1].Running, should.BeFalse)
			assert.Loosely(c, s.Invocations, should.HaveLength(3))

			// Make sure the policy didn't discard anything.
			assert.Loosely(c, s.DiscardedTriggers, should.HaveLength(0))
		})

		c.Run("Limited batching works", func(c *ftt.Test) {
			// A policy that allows two concurrent invocations with batch size 3.
			var err error
			s.Policy, err = GreedyBatchingPolicy(2, 3)
			assert.Loosely(c, err, should.BeNil)

			// Submit a bunch of triggers at once. They'll result in two invocations,
			// per batch size limits.
			s.AddTrigger(0,
				internal.NoopTrigger("t1", ""),
				internal.NoopTrigger("t2", ""),
				internal.NoopTrigger("t3", ""),
				internal.NoopTrigger("t4", ""))

			// Yep.
			assert.Loosely(c, s.PendingTriggers, should.HaveLength(0))
			assert.Loosely(c, s.Invocations, should.HaveLength(2))
			assert.Loosely(c, s.Invocations[0].Request.TriggerIDs(), should.Match([]string{"t1", "t2", "t3"}))
			assert.Loosely(c, s.Invocations[1].Request.TriggerIDs(), should.Match([]string{"t4"}))

			// Some time later while the invocations are still running, submit more
			// triggers.
			s.AddTrigger(30*time.Second,
				internal.NoopTrigger("t5", ""),
				internal.NoopTrigger("t6", ""),
				internal.NoopTrigger("t7", ""),
				internal.NoopTrigger("t8", ""))

			// Nothing happened.
			assert.Loosely(c, s.PendingTriggers, should.HaveLength(4))
			assert.Loosely(c, s.Invocations, should.HaveLength(2))

			// Wait until invocations finish. It will cause a triage and pending
			// triggers will be dispatched (unevenly again).
			s.AdvanceTime(40 * time.Second)

			// Yep.
			assert.Loosely(c, s.PendingTriggers, should.HaveLength(0))
			assert.Loosely(c, s.Invocations, should.HaveLength(4))
			assert.Loosely(c, s.Invocations[2].Request.TriggerIDs(), should.Match([]string{"t5", "t6", "t7"}))
			assert.Loosely(c, s.Invocations[3].Request.TriggerIDs(), should.Match([]string{"t8"}))

			// Make sure the policy didn't discard anything.
			assert.Loosely(c, s.DiscardedTriggers, should.HaveLength(0))
		})
	})
}
