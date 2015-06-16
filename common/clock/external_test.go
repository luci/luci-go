// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clock

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

// testClock is a Clock implementation used for testing.
type testClock struct {
	nowCallback      func() time.Time
	sleepCallback    func()
	newTimerCallback func() Timer
	afterCallback    func() <-chan time.Time
}

func (tc *testClock) Now() time.Time {
	return tc.nowCallback()
}

func (tc *testClock) Sleep(time.Duration) {
	tc.sleepCallback()
}

func (tc *testClock) NewTimer() Timer {
	return tc.newTimerCallback()
}

func (tc *testClock) After(time.Duration) <-chan time.Time {
	return tc.afterCallback()
}

func TestExternal(t *testing.T) {
	t.Parallel()

	now := time.Date(2015, 01, 01, 0, 0, 0, 0, time.UTC)
	Convey(`A Context with a testClock installed`, t, func() {
		tc := &testClock{}
		c := Set(context.Background(), tc)

		Convey(`Now() will use the testClock's Now().`, func() {
			used := false
			tc.nowCallback = func() time.Time {
				used = true
				return now
			}

			So(Now(c), ShouldResemble, now)
			So(used, ShouldBeTrue)
		})

		Convey(`Sleep() will use testClock's Sleep().`, func() {
			used := false
			tc.sleepCallback = func() {
				used = true
			}

			Sleep(c, time.Second)
			So(used, ShouldBeTrue)
		})

		Convey(`NewTimer() will use testClock's NewTimer().`, func() {
			used := false
			tc.newTimerCallback = func() Timer {
				used = true
				return nil
			}

			NewTimer(c)
			So(used, ShouldBeTrue)
		})

		Convey(`After() will use testClock's After().`, func() {
			used := false
			tc.afterCallback = func() <-chan time.Time {
				used = true
				return nil
			}

			After(c, time.Second)
			So(used, ShouldBeTrue)
		})
	})

	Convey(`An Context with no clock installed`, t, func() {
		c := context.Background()

		Convey(`Will return a SystemClock instance.`, func() {
			So(Get(c), ShouldHaveSameTypeAs, systemClock{})
		})
	})
}
