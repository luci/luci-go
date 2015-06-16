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

func TestTestClock(t *testing.T) {
	t.Parallel()

	Convey(`A testing clock instance`, t, func() {
		now := time.Date(2015, 01, 01, 00, 00, 00, 00, time.UTC)
		c := New(now)

		Convey(`Returns the current time.`, func() {
			So(c.Now(), ShouldResemble, now)
		})

		Convey(`When sleeping with a time of zero, immediately awakens.`, func() {
			c.Sleep(0)
			So(c.Now(), ShouldResemble, now)
		})

		Convey(`When sleeping for a period of time, awakens when signalled.`, func() {
			sleepingC := make(chan struct{})
			c.SetTimerCallback(func(_ clock.Timer) {
				close(sleepingC)
			})

			awakeC := make(chan time.Time)
			go func() {
				c.Sleep(2 * time.Second)
				awakeC <- c.Now()
			}()

			<-sleepingC
			c.Set(now.Add(1 * time.Second))
			c.Set(now.Add(2 * time.Second))
			So(<-awakeC, ShouldResemble, now.Add(2*time.Second))
		})

		Convey(`Awakens after a period of time.`, func() {
			afterC := c.After(2 * time.Second)

			c.Set(now.Add(1 * time.Second))
			c.Set(now.Add(2 * time.Second))
			So(<-afterC, ShouldResemble, now.Add(2*time.Second))
		})
	})
}
