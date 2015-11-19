// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundler

import (
	"sync"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTimeoutCond(t *testing.T) {
	Convey(`A timeout Cond bound to a lock`, t, func() {
		l := sync.Mutex{}
		tc := testclock.New(time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		c := newTimeoutCond(tc, &l)

		Convey(`Will wait without a timer if the duration is <0`, func() {
			didSetTimer := false
			tc.SetTimerCallback(func(time.Duration, clock.Timer) {
				didSetTimer = true
			})

			l.Lock()
			defer l.Unlock()

			// Signal the Cond until we unblock.
			go func() {
				l.Lock()
				defer l.Unlock()

				c.Signal()
			}()

			timeout := c.waitTimeout(-1)
			So(didSetTimer, ShouldBeFalse)
			So(timeout, ShouldBeFalse)
		})

		Convey(`Will timeout after the duration expires.`, func() {
			tc.SetTimerCallback(func(time.Duration, clock.Timer) {
				tc.Add(time.Second)
			})

			l.Lock()
			defer l.Unlock()

			timeout := c.waitTimeout(time.Second)
			So(timeout, ShouldBeTrue)
		})

		Convey(`Will not timeout if signalled before the timeout.`, func() {
			tc.SetTimerCallback(func(time.Duration, clock.Timer) {
				tc.Add(time.Second)
			})

			l.Lock()
			defer l.Unlock()

			t := tc.NewTimer()
			t.Reset(time.Second)
			go func() {
				<-t.GetC()
				c.Signal()
			}()

			timeout := c.waitTimeout(5 * time.Second)
			So(timeout, ShouldBeFalse)
		})
	})
}
