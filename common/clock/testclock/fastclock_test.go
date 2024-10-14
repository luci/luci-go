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
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func ExampleFastClock() {
	// New clock running 1M times faster than wall clock.
	clk := NewFastClock(TestRecentTimeUTC, 1000*1000)
	defer clk.Close()
	ctx := clock.Set(context.Background(), clk)
	wallStart := time.Now()
	t := clock.NewTimer(ctx)
	t.Reset(time.Hour)
	<-t.GetC()
	if wallTook := time.Since(wallStart); wallTook <= 10*time.Second {
		fmt.Printf("took less than 10 seconds\n")
		// Output: took less than 10 seconds
	} else {
		fmt.Printf("took %s, either due to a bug or CPU starvation\n", wallTook)
	}
}

func TestFastClock(t *testing.T) {
	t.Parallel()

	ftt.Run(`A FastClock instance`, t, func(t *ftt.Test) {
		epoch := time.Date(2015, 01, 01, 00, 00, 00, 00, time.UTC)
		t.Run(`Tied to a wall clock but running 1000 times faster`, func(t *ftt.Test) {
			fc := NewFastClock(epoch, 1000)
			defer fc.Close()
			ctx := clock.Set(context.Background(), fc)

			t.Run(`Returns the current time.`, func(t *ftt.Test) {
				wall0, fast0 := fc.now()
				fast1 := fc.Now()
				wall2, fast2 := fc.now()
				// Check assumptions of the wall clock, which should be monotonically
				// non-decreasing.
				assert.Loosely(t, wall2, should.HappenOnOrAfter(wall0))
				// Ditto for the FastClock.
				assert.Loosely(t, fast0, should.HappenOnOrAfter(epoch))
				assert.Loosely(t, fast1, should.HappenOnOrAfter(fast0))
				assert.Loosely(t, fast2, should.HappenOnOrAfter(fast1))

				t.Run(`Runs faster than the wall clock.`, func(t *ftt.Test) {
					assert.Loosely(t, fast2.Sub(fast0), should.Equal(wall2.Sub(wall0)*1000))
				})
			})

			t.Run(`When sleeping with a time of zero, awakens quickly.`, func(t *ftt.Test) {
				wall0, _ := fc.now()
				fc.Sleep(ctx, 0)
				wall1, _ := fc.now()
				// The smaller, the better, but allow for some CPU starvation.
				assert.Loosely(t, wall1.Sub(wall0), should.BeLessThan(time.Minute))
			})

			t.Run(`Will panic if going backwards in time via Add().`, func(t *ftt.Test) {
				assert.Loosely(t, func() { fc.Add(-1 * time.Second) }, should.Panic)
			})
			t.Run(`Will not panic if going backwards in time via Set().`, func(t *ftt.Test) {
				assert.Loosely(t, func() { fc.Set(fc.Now().Add(-1 * time.Second)) }, should.NotPanic)
			})

			t.Run(`When sleeping, awakens if canceled.`, func(t *ftt.Test) {
				ctx, cancelFunc := context.WithCancel(ctx)

				fc.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
					cancelFunc()
				})

				assert.Loosely(t, fc.Sleep(ctx, time.Hour).Incomplete(), should.BeTrue)
			})

			t.Run(`Awakens after a period of time while manipulated.`, func(t *ftt.Test) {
				t0 := fc.Now()
				afterC := clock.After(ctx, time.Second)
				t1 := fc.Now()
				for i := 0; i < 1000 && fc.Now().Sub(t1) < time.Second; i++ {
					fc.Add(time.Millisecond)
				}
				res := <-afterC
				// The timer must not wake up earlier than necessary.
				assert.Loosely(t, res.Time.Sub(t0), should.BeGreaterThanOrEqual(time.Second))
				// The timer may wake a bit later than possible, especially if the
				// process is starving of CPU, so give it 1 hour of (fake) grace time.
				assert.Loosely(t, res.Time.Sub(t1), should.BeLessThan(time.Second+time.Hour))
			})

			t.Run(`Awakens quickly without being manipulated due to passage of wall clock time.`, func(t *ftt.Test) {
				const wallDelay = time.Millisecond
				const fastDelay = time.Second // 1000x faster

				wall0, fast0 := fc.now()
				fastAfter := clock.After(ctx, fastDelay)

				wallAfter := time.After(wallDelay)
				<-wallAfter
				wall1, _ := fc.now()
				assert.Loosely(t, wall1.Sub(wall0), should.BeGreaterThan(wallDelay))

				fastAwoken := <-fastAfter
				fastOverhead := (fastAwoken.Sub(fast0) - fastDelay)
				wallOverhead := fastOverhead / 1000
				t.Log(" overhead:", "Wall", wallOverhead, "FastClock", fastOverhead)
				assert.Loosely(t, wallOverhead, should.BeGreaterThan(time.Duration(0))) // check sanity
				// The smaller, the better, but allow for some CPU starvation.
				assert.Loosely(t, wallOverhead, should.BeLessThan(time.Minute))
			})
		})

		t.Run(`Tied to a wall clock mock`, func(t *ftt.Test) {
			wallEpoch := time.Date(2020, 01, 01, 00, 00, 00, 00, time.UTC)
			fc := &FastClock{
				fastToSysRatio: 1000,
				initFastTime:   epoch,
				initSysTime:    wallEpoch,
				systemNow:      func() time.Time { return wallEpoch },
			}
			ctx := clock.Set(context.Background(), fc)

			t.Run(`When sleeping with a time of zero, awakens immediately.`, func(t *ftt.Test) {
				before := fc.Now()
				fc.Sleep(ctx, 0)
				assert.Loosely(t, fc.Now(), should.Match(before))
			})

			t.Run(`When sleeping for a period of time, awakens when signalled.`, func(t *ftt.Test) {
				sleepingC := make(chan struct{})
				fc.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
					close(sleepingC)
				})

				awakeC := make(chan time.Time)
				go func() {
					fc.Sleep(ctx, 2*time.Second)
					awakeC <- fc.Now()
				}()

				<-sleepingC
				fc.Set(epoch.Add(1 * time.Second))
				fc.Set(epoch.Add(2 * time.Second))
				assert.Loosely(t, <-awakeC, should.Resemble(epoch.Add(2*time.Second)))
			})
		})
	})
}
