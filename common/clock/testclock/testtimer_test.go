// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testclock

import (
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTestTimer(t *testing.T) {
	t.Parallel()

	Convey(`A testing clock instance`, t, func() {
		now := time.Date(2015, 01, 01, 00, 00, 00, 00, time.UTC)
		c := New(now)

		Convey(`A timer instance`, func() {
			t := c.NewTimer()

			Convey(`Should have a non-nil C.`, func() {
				So(t.GetC(), ShouldBeNil)
			})

			Convey(`When activated`, func() {
				So(t.Reset(1*time.Second), ShouldBeFalse)

				Convey(`When reset, should return active.`, func() {
					So(t.Reset(1*time.Hour), ShouldBeTrue)
					So(t.GetC(), ShouldNotBeNil)
				})

				Convey(`When stopped, should return active.`, func() {
					So(t.Stop(), ShouldBeTrue)
					So(t.GetC(), ShouldBeNil)

					Convey(`And when stopped again, should return inactive.`, func() {
						So(t.Stop(), ShouldBeFalse)
						So(t.GetC(), ShouldBeNil)
					})
				})

				Convey(`When stopped after expiring, should not have a signal.`, func() {
					c.Add(1 * time.Second)
					So(t.Stop(), ShouldBeTrue)

					var signalled bool
					select {
					case <-t.GetC():
						signalled = true
					default:
						break
					}
					So(signalled, ShouldBeFalse)
				})
			})

			Convey(`Should successfully signal.`, func() {
				So(t.Reset(1*time.Second), ShouldBeFalse)
				c.Add(1 * time.Second)

				So(t.GetC(), ShouldNotBeNil)
				So(<-t.GetC(), ShouldResemble, now.Add(1*time.Second))
			})
		})

		Convey(`Multiple goroutines using the timer...`, func() {
			// Mark when timers are started, so we can ensure that our signalling
			// happens after the timers have been instantiated.
			timerStartedC := make(chan bool)
			c.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
				timerStartedC <- true
			})

			resultC := make(chan time.Time)
			for i := time.Duration(0); i < 5; i++ {
				go func(d time.Duration) {
					timer := c.NewTimer()
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
			c.Set(now.Add(2 * time.Second))
			<-resultC
			<-resultC
			<-resultC
			So(moreResults(), ShouldBeFalse)

			// Advance clock to +3s. One timer should signal.
			c.Set(now.Add(3 * time.Second))
			<-resultC
			So(moreResults(), ShouldBeFalse)

			// Advance clock to +10s. One final timer should signal.
			c.Set(now.Add(10 * time.Second))
			<-resultC
			So(moreResults(), ShouldBeFalse)
		})
	})
}
