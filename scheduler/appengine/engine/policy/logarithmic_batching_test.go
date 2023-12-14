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

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogarithmicBatching(t *testing.T) {
	t.Parallel()

	Convey("With simulator", t, func(c C) {
		invocationDuration := time.Hour // may be modified in tests below.
		s := Simulator{
			OnRequest: func(s *Simulator, r task.Request) time.Duration {
				return invocationDuration
			},
			OnDebugLog: func(format string, args ...any) {
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
				So(s.DiscardedTriggers, ShouldHaveLength, 0)
			})

			Convey("rounds down number of consumed triggers", func() {
				addTriggers(noDelay, 3)
				So(s.Invocations, ShouldHaveLength, 1)
				// log(2,3) = 1.584
				So(s.Last().Request.TriggerIDs(), ShouldResemble, []string{"t-001"})
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, "data-001")
				So(s.PendingTriggers, ShouldHaveLength, 2)
				So(s.DiscardedTriggers, ShouldHaveLength, 0)
			})

			Convey("respects maxBatchSize", func() {
				N := 1 << (maxBatchSize + 2)
				addTriggers(noDelay, N)
				So(s.Invocations, ShouldHaveLength, 1)
				So(s.Last().Request.TriggerIDs(), ShouldHaveLength, maxBatchSize)
				So(s.Last().Request.StringProperty("noop_trigger_data"), ShouldEqual, fmt.Sprintf("data-%03d", maxBatchSize))
				So(s.PendingTriggers, ShouldHaveLength, N-maxBatchSize)
				So(s.DiscardedTriggers, ShouldHaveLength, 0)
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
				So(s.DiscardedTriggers, ShouldHaveLength, 0)
			})
		})

		Convey("Long simulation", func() {
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
			So(err, ShouldBeNil)
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
			So(s.DiscardedTriggers, ShouldHaveLength, 0)

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
			_, _ = Printf("\n\n%s\nReport\n%s\n", separatorLine, separatorLine)
			_, _ = Printf(" * logBase             %.5f\n", logBase)
			_, _ = Printf(" * maxBatchSize        %d\n", maxBatchSize)
			_, _ = Printf(" * maxConcurrentBuilds %d\n", maxConcurrentBuilds)
			_, _ = Printf(" * build duration      %s\n", buildDuration)
			_, _ = Printf(" * simulation duration %s\n", simulDuration)
			_, _ = Println()
			_, _ = Printf("Simulated %d commits, %d builds, %d triggers remaining\n", lastAddedTrigger, len(s.Invocations), len(s.PendingTriggers))
			_, _ = Println()
			_, _ = Println(fmtPercentiles("    number of per-build commits", "%3d", triggerSizes, percentiles...))
			_, _ = Println(fmtPercentiles("      number of pending commits", "%3d", pendingSizes, percentiles...))
			_, _ = Println(fmtPercentiles("oldest pending commit (minutes)", "%3d", oldestCommitAgeMinutes, percentiles...))
			_, _ = Printf("%s\n", separatorLine)
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
