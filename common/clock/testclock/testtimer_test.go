// Copyright 2015 The LUCI Authors.
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

package testclock

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// trashTimer is a useless implementation of clock.Timer specifically designed
// to exist and not be a test timer type.
type trashTimer struct {
	clock.Timer
}

func TestTestTimer(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing clock instance`, t, func(t *ftt.Test) {
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		now := TestTimeLocal
		clk := New(now)

		t.Run(`A timer instance`, func(t *ftt.Test) {
			timer := clk.NewTimer(ctx)

			t.Run(`Should have a non-nil C.`, func(t *ftt.Test) {
				assert.Loosely(t, timer.GetC(), should.NotBeNil)
			})

			t.Run(`When activated`, func(t *ftt.Test) {
				assert.Loosely(t, timer.Reset(1*time.Second), should.BeFalse)

				t.Run(`When reset, should return active.`, func(t *ftt.Test) {
					assert.Loosely(t, timer.Reset(1*time.Hour), should.BeTrue)
					assert.Loosely(t, timer.GetC(), should.NotBeNil)
				})

				t.Run(`When stopped, should return active.`, func(t *ftt.Test) {
					assert.Loosely(t, timer.Stop(), should.BeTrue)
					assert.Loosely(t, timer.GetC(), should.NotBeNil)

					t.Run(`And when stopped again, should return inactive.`, func(t *ftt.Test) {
						assert.Loosely(t, timer.Stop(), should.BeFalse)
						assert.Loosely(t, timer.GetC(), should.NotBeNil)
					})
				})

				t.Run(`When stopped after expiring, will return false.`, func(t *ftt.Test) {
					clk.Add(1 * time.Second)
					assert.Loosely(t, timer.Stop(), should.BeFalse)

					var signalled bool
					select {
					case <-timer.GetC():
						signalled = true
					default:
						break
					}
					assert.Loosely(t, signalled, should.BeTrue)
				})
			})

			t.Run(`Should successfully signal.`, func(t *ftt.Test) {
				assert.Loosely(t, timer.Reset(1*time.Second), should.BeFalse)
				clk.Add(1 * time.Second)

				assert.Loosely(t, <-timer.GetC(), should.Match(clock.TimerResult{Time: now.Add(1 * time.Second)}))
			})

			t.Run(`Should signal immediately if the timer is in the past.`, func(t *ftt.Test) {
				assert.Loosely(t, timer.Reset(-1*time.Second), should.BeFalse)
				assert.Loosely(t, <-timer.GetC(), should.Match(clock.TimerResult{Time: now}))
			})

			t.Run(`Will trigger immediately if the Context is canceled when reset.`, func(t *ftt.Test) {
				cancelFunc()

				// Works for the first timer?
				assert.Loosely(t, timer.Reset(time.Hour), should.BeFalse)
				assert.Loosely(t, (<-timer.GetC()).Err, should.Equal(context.Canceled))

				// Works for the second timer?
				assert.Loosely(t, timer.Reset(time.Hour), should.BeFalse)
				assert.Loosely(t, (<-timer.GetC()).Err, should.Equal(context.Canceled))
			})

			t.Run(`Will trigger when the Context is canceled.`, func(t *ftt.Test) {
				clk.SetTimerCallback(func(time.Duration, clock.Timer) {
					cancelFunc()
				})

				// Works for the first timer?
				assert.Loosely(t, timer.Reset(time.Hour), should.BeFalse)
				assert.Loosely(t, (<-timer.GetC()).Err, should.Equal(context.Canceled))

				// Works for the second timer?
				assert.Loosely(t, timer.Reset(time.Hour), should.BeFalse)
				assert.Loosely(t, (<-timer.GetC()).Err, should.Equal(context.Canceled))
			})

			t.Run(`Will use the same channel when reset.`, func(t *ftt.Test) {
				timerC := timer.GetC()
				timer.Reset(time.Second)
				clk.Add(time.Second)
				assert.Loosely(t, <-timerC, should.Match(clock.TimerResult{Time: now.Add(1 * time.Second)}))
			})

			t.Run(`Will not signal the timer channel if stopped.`, func(t *ftt.Test) {
				timer.Reset(time.Second)
				assert.Loosely(t, timer.Stop(), should.BeTrue)
				clk.Add(time.Second)

				triggered := false
				select {
				case <-timer.GetC():
					triggered = true
				default:
					break
				}
				assert.Loosely(t, triggered, should.BeFalse)
			})

			t.Run(`Will not trigger on previous time thresholds if reset.`, func(t *ftt.Test) {
				timer.Reset(time.Second)
				timer.Reset(2 * time.Second)
				clk.Add(time.Second)
				clk.Add(time.Second)
				assert.Loosely(t, (<-timer.GetC()).Time, should.Match(clk.Now()))
			})

			t.Run(`Can set and retrieve timer tags.`, func(t *ftt.Test) {
				var tagMu sync.Mutex
				tagMap := map[string]struct{}{}

				// On the last timer callback, advance time past the timer threshold.
				timers := make([]clock.Timer, 10)
				count := 0
				clk.SetTimerCallback(func(_ time.Duration, timer clock.Timer) {
					tagMu.Lock()
					defer tagMu.Unlock()

					tagMap[strings.Join(GetTags(timer), "")] = struct{}{}
					count++
					if count == len(timers) {
						clk.Add(time.Second)
					}
				})

				for i := range timers {
					timer := clk.NewTimer(clock.Tag(ctx, fmt.Sprintf("%d", i)))
					timer.Reset(time.Second)
					timers[i] = timer
				}

				// Wait until all timers have expired.
				for _, timer := range timers {
					<-timer.GetC()
				}

				// Did we see all of their tags?
				for i := range timers {
					_, ok := tagMap[fmt.Sprintf("%d", i)]
					assert.Loosely(t, ok, should.BeTrue)
				}
			})

			t.Run(`A non-test timer has a nil tag.`, func(t *ftt.Test) {
				tTimer := trashTimer{}
				assert.Loosely(t, GetTags(tTimer), should.BeNil)
			})
		})

		t.Run(`Multiple goroutines using timers...`, func(t *ftt.Test) {
			// Mark when timers are started, so we can ensure that our signalling
			// happens after the timers have been instantiated.
			timerStartedC := make(chan bool)
			clk.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
				timerStartedC <- true
			})

			resultC := make(chan clock.TimerResult)
			for i := range time.Duration(5) {
				go func(d time.Duration) {
					timer := clk.NewTimer(ctx)
					timer.Reset(d)
					resultC <- <-timer.GetC()
				}(i * time.Second)
				<-timerStartedC
			}

			moreResults := func() bool {
				select {
				case <-resultC:
					return true
				default:
					return false
				}
			}

			// Advance the clock to +2s. Three timers should signal.
			clk.Set(now.Add(2 * time.Second))
			<-resultC
			<-resultC
			<-resultC
			assert.Loosely(t, moreResults(), should.BeFalse)

			// Advance clock to +3s. One timer should signal.
			clk.Set(now.Add(3 * time.Second))
			<-resultC
			assert.Loosely(t, moreResults(), should.BeFalse)

			// Advance clock to +10s. One final timer should signal.
			clk.Set(now.Add(10 * time.Second))
			<-resultC
			assert.Loosely(t, moreResults(), should.BeFalse)
		})
	})
}

func TestTimerTags(t *testing.T) {
	t.Parallel()

	ftt.Run(`A context with tags {"A", "B"}`, t, func(t *ftt.Test) {
		c := clock.Tag(clock.Tag(context.Background(), "A"), "B")

		t.Run(`Has tags, {"A", "B"}`, func(t *ftt.Test) {
			assert.Loosely(t, clock.Tags(c), should.Match([]string{"A", "B"}))
		})

		t.Run(`Will be retained by a testclock.Timer.`, func(t *ftt.Test) {
			tc := New(TestTimeUTC)
			timer := tc.NewTimer(c)

			assert.Loosely(t, GetTags(timer), should.Match([]string{"A", "B"}))

			t.Run(`The timer tests positive for tags {"A", "B"}`, func(t *ftt.Test) {
				assert.Loosely(t, HasTags(timer, "A", "B"), should.BeTrue)
			})

			t.Run(`The timer tests negative for tags {"A"}, {"B"}, and {"A", "C"}`, func(t *ftt.Test) {
				assert.Loosely(t, HasTags(timer, "A"), should.BeFalse)
				assert.Loosely(t, HasTags(timer, "B"), should.BeFalse)
				assert.Loosely(t, HasTags(timer, "A", "C"), should.BeFalse)
			})
		})

		t.Run(`A non-test timer tests negative for {"A"} and {"A", "B"}.`, func(t *ftt.Test) {
			tTimer := &trashTimer{}

			assert.Loosely(t, HasTags(tTimer, "A"), should.BeFalse)
			assert.Loosely(t, HasTags(tTimer, "A", "B"), should.BeFalse)
		})
	})
}
