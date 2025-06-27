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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

func TestNewestFirst(t *testing.T) {
	t.Parallel()

	ftt.Run("With simulator", t, func(t *ftt.Test) {
		invocationDuration := time.Hour // may be modified in tests below.
		s := Simulator{
			OnRequest: func(s *Simulator, r task.Request) time.Duration {
				return invocationDuration
			},
			OnDebugLog: func(format string, args ...any) {
				t.Logf(format+"\n", args...)
			},
		}

		const noDelay = time.Duration(0)
		lastAddedTrigger := 0
		addTriggers := func(delay time.Duration, n int) {
			ts := make([]*internal.Trigger, n)
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

		t.Run("Pending timeout must be positive", func(t *ftt.Test) {
			s.Policy, err = NewestFirstPolicy(2, -time.Hour)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Newest first works", func(t *ftt.Test) {
			t.Run("at least 1 trigger", func(t *ftt.Test) {
				s.Policy, err = NewestFirstPolicy(1, 300*invocationDuration)
				assert.Loosely(t, err, should.BeNil)

				// Add exactly one trigger.
				addTriggers(noDelay, 1)
				assert.Loosely(t, s.Invocations, should.HaveLength(1))
				assert.Loosely(t, s.Last().Request.TriggerIDs(), should.Match([]string{"t-001"}))
				assert.Loosely(t, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal("data-001"))
				assert.Loosely(t, s.PendingTriggers, should.HaveLength(0))
				assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(0))
			})

			t.Run("process newer triggers first", func(t *ftt.Test) {
				s.Policy, err = NewestFirstPolicy(1, 300*invocationDuration)
				assert.Loosely(t, err, should.BeNil)

				const N = 3
				addTriggers(noDelay, N)
				for i := N - 1; i >= 0; i-- {
					assert.Loosely(t, s.Invocations, should.HaveLength(N-i))
					assert.Loosely(t, s.Last().Request.TriggerIDs(), should.Match([]string{fmt.Sprintf("t-%03d", i+1)}))
					assert.Loosely(t, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal(fmt.Sprintf("data-%03d", i+1)))
					assert.Loosely(t, s.PendingTriggers, should.HaveLength(i))
					assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(0))
					addTriggers(invocationDuration, 0)
				}
				assert.Loosely(t, s.Invocations, should.HaveLength(N))
				assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(0))
			})

			t.Run("respects pending timeout", func(t *ftt.Test) {
				const N = 2
				const extra = 2
				s.Policy, err = NewestFirstPolicy(1, N*invocationDuration)
				assert.Loosely(t, err, should.BeNil)

				// Add more extra triggers than we can fit running serially in the pending timeout.
				// This extra trigger will be discarded.
				addTriggers(noDelay, N+extra)

				assert.Loosely(t, s.Invocations, should.HaveLength(1))
				assert.Loosely(t, s.Last().Request.TriggerIDs(), should.Match([]string{"t-004"}))
				assert.Loosely(t, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal("data-004"))
				assert.Loosely(t, s.PendingTriggers, should.HaveLength(1+extra))
				assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(0))

				// Advance time, allowing an invocation to finish.
				addTriggers(invocationDuration, 0)

				assert.Loosely(t, s.Invocations, should.HaveLength(2))
				assert.Loosely(t, s.Last().Request.TriggerIDs(), should.Match([]string{"t-003"}))
				assert.Loosely(t, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal("data-003"))
				assert.Loosely(t, s.PendingTriggers, should.HaveLength(extra))
				assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(0))

				// Advance time, allowing an invocation to finish.
				addTriggers(invocationDuration, 0)

				assert.Loosely(t, s.Invocations, should.HaveLength(N))
				assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(extra))
			})

			t.Run("pending timeout discards triggers due to starvation", func(t *ftt.Test) {
				const N = 3
				s.Policy, err = NewestFirstPolicy(1, N*invocationDuration)
				assert.Loosely(t, err, should.BeNil)

				// Add more extra triggers than we can fit running serially in the pending timeout.
				// This extra trigger will be discarded.
				extra := 1
				addTriggers(noDelay, 1+extra)
				for i := range N {
					assert.Loosely(t, s.Invocations, should.HaveLength(i+1))
					assert.Loosely(t, s.Last().Request.TriggerIDs(), should.Match([]string{fmt.Sprintf("t-%03d", i+1+extra)}))
					assert.Loosely(t, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal(fmt.Sprintf("data-%03d", i+1+extra)))
					assert.Loosely(t, s.PendingTriggers, should.HaveLength(1))
					assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(0))

					// Add the trigger first. We want a newer trigger to always come in before a slot frees up so
					// that the extra triggers get starved.
					addTriggers(invocationDuration/2, 1)
					addTriggers(invocationDuration/2, 0)
				}
				assert.Loosely(t, s.Invocations, should.HaveLength(N+1))
				assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(extra))
			})

			t.Run("very short timeout causes immediate discard", func(t *ftt.Test) {
				s.Policy, err = NewestFirstPolicy(1, time.Nanosecond)
				assert.Loosely(t, err, should.BeNil)

				for i := range 3 {
					addTriggers(invocationDuration, 2)
					assert.Loosely(t, s.Invocations, should.HaveLength(i+1))
					assert.Loosely(t, s.PendingTriggers, should.HaveLength(1))
					assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(i))
				}
			})

			t.Run("multiple concurrent invocations", func(t *ftt.Test) {
				const concurrentInvocations = 2
				s.Policy, err = NewestFirstPolicy(concurrentInvocations, 2*invocationDuration)
				assert.Loosely(t, err, should.BeNil)

				addTriggers(noDelay, 6)

				assert.Loosely(t, s.Invocations, should.HaveLength(2))
				assert.Loosely(t, s.PendingTriggers, should.HaveLength(4))
				assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(0))

				addTriggers(invocationDuration, 2)

				assert.Loosely(t, s.Invocations, should.HaveLength(4))
				assert.Loosely(t, s.PendingTriggers, should.HaveLength(4))
				assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(0))

				addTriggers(invocationDuration, 2)

				assert.Loosely(t, s.Invocations, should.HaveLength(6))
				assert.Loosely(t, s.PendingTriggers, should.HaveLength(2))
				assert.Loosely(t, s.DiscardedTriggers, should.HaveLength(2))
			})
		})
	})
}
