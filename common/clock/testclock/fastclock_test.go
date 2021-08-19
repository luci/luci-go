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

	. "github.com/smartystreets/goconvey/convey"
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

	Convey(`A FastClock instance`, t, func() {
		epoch := time.Date(2015, 01, 01, 00, 00, 00, 00, time.UTC)
		Convey(`Tied to a wall clock but running 1000 times faster`, func() {
			fc := NewFastClock(epoch, 1000)
			defer fc.Close()
			ctx := clock.Set(context.Background(), fc)

			Convey(`Returns the current time.`, func() {
				wall0, fast0 := fc.now()
				fast1 := fc.Now()
				wall2, fast2 := fc.now()
				// Check assumptions of the wall clock, which should be monotonically
				// non-decreasing.
				So(wall2, ShouldHappenOnOrAfter, wall0)
				// Ditto for the FastClock.
				So(fast0, ShouldHappenOnOrAfter, epoch)
				So(fast1, ShouldHappenOnOrAfter, fast0)
				So(fast2, ShouldHappenOnOrAfter, fast1)

				Convey(`Runs faster than the wall clock.`, func() {
					So(fast2.Sub(fast0), ShouldAlmostEqual, wall2.Sub(wall0)*1000)
				})
			})

			Convey(`When sleeping with a time of zero, awakens quickly.`, func() {
				wall0, _ := fc.now()
				fc.Sleep(ctx, 0)
				wall1, _ := fc.now()
				// The smaller, the better, but allow for some CPU starvation.
				So(wall1.Sub(wall0), ShouldBeLessThan, time.Minute)
			})

			Convey(`Will panic if going backwards in time.`, func() {
				So(func() { fc.Add(-1 * time.Second) }, ShouldPanic)
				So(func() { fc.Set(fc.Now().Add(-1 * time.Second)) }, ShouldPanic)
			})

			Convey(`When sleeping, awakens if canceled.`, func() {
				ctx, cancelFunc := context.WithCancel(ctx)

				fc.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
					cancelFunc()
				})

				So(fc.Sleep(ctx, time.Hour).Incomplete(), ShouldBeTrue)
			})

			Convey(`Awakens after a period of time while manipulated.`, func() {
				t0 := fc.Now()
				afterC := clock.After(ctx, time.Second)
				t1 := fc.Now()
				for i := 0; i < 1000 && fc.Now().Sub(t1) < time.Second; i++ {
					fc.Add(time.Millisecond)
				}
				res := <-afterC
				// The timer must not wake up earlier than necessary.
				So(res.Time.Sub(t0), ShouldBeGreaterThanOrEqualTo, time.Second)
				// The timer may wake a bit later than possible, especially if the
				// process is starving of CPU, so give it 1 hour of (fake) grace time.
				So(res.Time.Sub(t1), ShouldBeLessThan, time.Second+time.Hour)
			})

			Convey(`Awakens quickly without being manipulated due to passage of wall clock time.`, func() {
				const wallDelay = time.Millisecond
				const fastDelay = time.Second // 1000x faster

				wall0, fast0 := fc.now()
				fastAfter := clock.After(ctx, fastDelay)

				wallAfter := time.After(wallDelay)
				<-wallAfter
				wall1, _ := fc.now()
				So(wall1.Sub(wall0), ShouldBeGreaterThan, wallDelay)

				fastAwoken := <-fastAfter
				fastOverhead := (fastAwoken.Sub(fast0) - fastDelay)
				wallOverhead := fastOverhead / 1000
				_, _ = Println(" overhead:", "Wall", wallOverhead, "FastClock", fastOverhead)
				So(wallOverhead, ShouldBeGreaterThan, time.Duration(0)) // check sanity
				// The smaller, the better, but allow for some CPU starvation.
				So(wallOverhead, ShouldBeLessThan, time.Minute)
			})
		})

		Convey(`Tied to a wall clock mock`, func() {
			wallEpoch := time.Date(2020, 01, 01, 00, 00, 00, 00, time.UTC)
			fc := &FastClock{
				fastToSysRatio: 1000,
				initFastTime:   epoch,
				initSysTime:    wallEpoch,
				systemNow:      func() time.Time { return wallEpoch },
			}
			ctx := clock.Set(context.Background(), fc)

			Convey(`When sleeping with a time of zero, awakens immediately.`, func() {
				before := fc.Now()
				fc.Sleep(ctx, 0)
				So(fc.Now(), ShouldEqual, before)
			})

			Convey(`When sleeping for a period of time, awakens when signalled.`, func() {
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
				So(<-awakeC, ShouldResemble, epoch.Add(2*time.Second))
			})
		})
	})
}
