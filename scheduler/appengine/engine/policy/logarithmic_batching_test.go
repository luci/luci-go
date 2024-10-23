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
	"sort"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

func TestLogarithmicBatching(t *testing.T) {
	t.Parallel()

	ftt.Run("With simulator", t, func(c *ftt.Test) {
		invocationDuration := time.Hour // may be modified in tests below.
		s := Simulator{
			OnRequest: func(s *Simulator, r task.Request) time.Duration {
				return invocationDuration
			},
			OnDebugLog: func(format string, args ...any) {
				c.Logf(format+"\n", args...)
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

		c.Run("Logarithm base must be at least 1.0001", func(c *ftt.Test) {
			s.Policy, err = LogarithmicBatchingPolicy(2, 1000, 1.0)
			assert.Loosely(c, err, should.NotBeNil)
		})

		c.Run("Logarithmic batching works", func(c *ftt.Test) {
			// A policy that allows 1 concurrent invocation with effectively unlimited
			// batch size and logarithm base of 2.
			const maxBatchSize = 5
			s.Policy, err = LogarithmicBatchingPolicy(1, maxBatchSize, 2.0)
			assert.Loosely(c, err, should.BeNil)

			c.Run("at least 1 trigger", func(c *ftt.Test) {
				// Add exactly one trigger; log(2,1) == 0.
				addTriggers(noDelay, 1)
				assert.Loosely(c, s.Invocations, should.HaveLength(1))
				assert.Loosely(c, s.Last().Request.TriggerIDs(), should.Resemble([]string{"t-001"}))
				assert.Loosely(c, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal("data-001"))
				assert.Loosely(c, s.PendingTriggers, should.HaveLength(0))
				assert.Loosely(c, s.DiscardedTriggers, should.HaveLength(0))
			})

			c.Run("rounds down number of consumed triggers", func(c *ftt.Test) {
				addTriggers(noDelay, 3)
				assert.Loosely(c, s.Invocations, should.HaveLength(1))
				// log(2,3) = 1.584
				assert.Loosely(c, s.Last().Request.TriggerIDs(), should.Resemble([]string{"t-001"}))
				assert.Loosely(c, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal("data-001"))
				assert.Loosely(c, s.PendingTriggers, should.HaveLength(2))
				assert.Loosely(c, s.DiscardedTriggers, should.HaveLength(0))
			})

			c.Run("respects maxBatchSize", func(c *ftt.Test) {
				N := 1 << (maxBatchSize + 2)
				addTriggers(noDelay, N)
				assert.Loosely(c, s.Invocations, should.HaveLength(1))
				assert.Loosely(c, s.Last().Request.TriggerIDs(), should.HaveLength(maxBatchSize))
				assert.Loosely(c, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal(fmt.Sprintf("data-%03d", maxBatchSize)))
				assert.Loosely(c, s.PendingTriggers, should.HaveLength(N-maxBatchSize))
				assert.Loosely(c, s.DiscardedTriggers, should.HaveLength(0))
			})

			c.Run("Many triggers", func(c *ftt.Test) {
				// Add 5 triggers.
				addTriggers(noDelay, 5)
				assert.Loosely(c, s.Invocations, should.HaveLength(1))
				assert.Loosely(c, s.Last().Request.TriggerIDs(), should.Resemble([]string{"t-001", "t-002"}))
				assert.Loosely(c, s.Last().Request.StringProperty("noop_trigger_data"), should.Equal("data-002"))
				assert.Loosely(c, s.PendingTriggers, should.HaveLength(3))

				// Add a few triggers while the invocation is running.
				addTriggers(invocationDuration/4, 2)
				addTriggers(invocationDuration/4, 2)
				addTriggers(invocationDuration/4, 2)
				addTriggers(invocationDuration/4, 2)
				// Invocation is finsihed now, we have 11 = (3 old + 4*2 new triggers).
				assert.Loosely(c, s.Invocations, should.HaveLength(2)) // new invocation created.
				// log(2,11) = 3.459
				assert.Loosely(c, s.Last().Request.TriggerIDs(), should.Resemble([]string{"t-003", "t-004", "t-005"}))
				assert.Loosely(c, s.DiscardedTriggers, should.HaveLength(0))
			})
		})

		c.Run("Long simulation", func(c *ftt.Test) {
			// Run this with: `go test -run TestLogarithmicBatching -v`
			// TODO(tandrii): maybe make it like a Go benchmark?

			// Parameters.
			const (
				veryVerbose         = false
				maxBatchSize        = 50
				maxConcurrentBuilds = 2
				logBase             = 1.185
				buildDuration       = 43 * time.Minute
				simulDuration       = time.Hour * 24 * 5
			)
			percentiles := []int{0, 25, 50, 75, 100}
			// Value of 10 below means a commit lands every 10 minutes.
			MinutesBetweenCommitsByHourOfDay := []int{
				20, 30, 30, 30, 30, 30, // midnight .. 6am
				20, 10, 6, 3, 1, 1, // 6am..noon
				1, 1, 2, 3, 4, 5, // noon .. 6pm
				6, 10, 12, 15, 20, 20, // 6pm .. midnight
			}

			// Setup.
			invocationDuration = buildDuration
			s.Policy, err = LogarithmicBatchingPolicy(maxConcurrentBuilds, maxBatchSize, logBase)
			assert.Loosely(c, err, should.BeNil)
			s.Epoch = testclock.TestRecentTimeUTC.Truncate(24 * time.Hour)

			// Simulate.
			var pendingSizes []int
			var commits []time.Time
			for {
				delay := time.Minute * time.Duration(MinutesBetweenCommitsByHourOfDay[s.Now.Hour()])
				if delay <= 0 {
					panic("wrong minutesOfCommitDelayByHourOfDay")
				}
				addTriggers(delay, 1)
				commits = append(commits, s.Now)
				pendingSizes = append(pendingSizes, len(s.PendingTriggers))

				elapsed := s.Now.Sub(s.Epoch)
				if veryVerbose {
					s.OnDebugLog("%70s [%12s]  [%s] remaining %d", "", elapsed, s.Now, len(s.PendingTriggers))
				}
				if elapsed > simulDuration {
					break
				}
			}

			// There should never be any discarded triggers with this policy.
			assert.Loosely(c, s.DiscardedTriggers, should.HaveLength(0))

			// Analyze.
			var oldestCommitAgeMinutes []int
			triggerSizes := make([]int, len(s.Invocations))
			commistProcesssed := 0
			for i, inv := range s.Invocations {
				l := len(inv.Request.IncomingTriggers)
				oldestCreatedAt := commits[commistProcesssed]
				oldestAge := s.Epoch.Add(inv.Created).Sub(oldestCreatedAt)
				commistProcesssed += l
				triggerSizes[i] = l
				oldestCommitAgeMinutes = append(oldestCommitAgeMinutes, int(oldestAge/time.Minute))
			}

			separatorLine := strings.Repeat("=", 80)
			t.Logf("\n\n%s\nReport\n%s\n", separatorLine, separatorLine)
			t.Logf(" * logBase             %.5f\n", logBase)
			t.Logf(" * maxBatchSize        %d\n", maxBatchSize)
			t.Logf(" * maxConcurrentBuilds %d\n", maxConcurrentBuilds)
			t.Logf(" * build duration      %s\n", buildDuration)
			t.Logf(" * simulation duration %s\n", simulDuration)
			t.Log()
			t.Logf("Simulated %d commits, %d builds, %d triggers remaining\n", lastAddedTrigger, len(s.Invocations), len(s.PendingTriggers))
			t.Log()
			t.Log(fmtPercentiles("    number of per-build commits", "%3d", triggerSizes, percentiles...))
			t.Log(fmtPercentiles("      number of pending commits", "%3d", pendingSizes, percentiles...))
			t.Log(fmtPercentiles("oldest pending commit (minutes)", "%3d", oldestCommitAgeMinutes, percentiles...))
			t.Logf("%s\n", separatorLine)
		})
	})
}

func fmtPercentiles(prefix, valFormat string, values []int, percentiles ...int) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteRune(':')

	sort.Ints(values)
	sort.Ints(percentiles)
	l := len(values)
	for _, p := range percentiles {
		switch {
		case p < 0 || p > 100:
			panic(fmt.Errorf("invalid percentile %d, must be in 0..100", p))
		case p == 0:
			sb.WriteString(" min ")
		case p == 100:
			sb.WriteString(" max ")
		default:
			_, _ = fmt.Fprintf(&sb, "  p%02d ", p)
		}
		idx := l * p / 100
		if idx >= l {
			idx = l - 1
		}
		_, _ = fmt.Fprintf(&sb, valFormat, values[idx])
		sb.WriteRune(',')
	}

	return strings.TrimRight(sb.String(), ",")
}
