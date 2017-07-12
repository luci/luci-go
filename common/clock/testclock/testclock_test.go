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
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestTestClock(t *testing.T) {
	t.Parallel()

	Convey(`A testing clock instance`, t, func() {
		now := time.Date(2015, 01, 01, 00, 00, 00, 00, time.UTC)
		ctx, clk := UseTime(context.Background(), now)

		Convey(`Returns the current time.`, func() {
			So(clk.Now(), ShouldResemble, now)
		})

		Convey(`When sleeping with a time of zero, immediately awakens.`, func() {
			clk.Sleep(ctx, 0)
			So(clk.Now(), ShouldResemble, now)
		})

		Convey(`Will panic if going backwards in time.`, func() {
			So(func() { clk.Add(-1 * time.Second) }, ShouldPanic)
		})

		Convey(`When sleeping for a period of time, awakens when signalled.`, func() {
			sleepingC := make(chan struct{})
			clk.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
				close(sleepingC)
			})

			awakeC := make(chan time.Time)
			go func() {
				clk.Sleep(ctx, 2*time.Second)
				awakeC <- clk.Now()
			}()

			<-sleepingC
			clk.Set(now.Add(1 * time.Second))
			clk.Set(now.Add(2 * time.Second))
			So(<-awakeC, ShouldResemble, now.Add(2*time.Second))
		})

		Convey(`Awakens after a period of time.`, func() {
			afterC := clk.After(ctx, 2*time.Second)

			clk.Set(now.Add(1 * time.Second))
			clk.Set(now.Add(2 * time.Second))
			So(<-afterC, ShouldResemble, clock.TimerResult{now.Add(2 * time.Second), nil})
		})

		Convey(`When sleeping, awakens if canceled.`, func() {
			ctx, cancelFunc := context.WithCancel(ctx)

			clk.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
				cancelFunc()
			})

			So(clk.Sleep(ctx, time.Second).Incomplete(), ShouldBeTrue)
		})
	})
}
