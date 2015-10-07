// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testclock

import (
	"time"

	"github.com/luci/luci-go/common/clock"
)

// timer is an implementation of clock.TestTimer that uses a channel
// to signal the timer to fire.
//
// The channel is buffered so it can be used without requiring a separate
// signalling goroutine.
type timer struct {
	clock   *testClock
	signalC chan time.Time

	// Cancels callback from clock.invokeAt. Being not nil implies that the timer
	// is active.
	cancelFunc cancelFunc
}

var _ clock.Timer = (*timer)(nil)

// NewTimer returns a new, instantiated timer.
func newTimer(clock *testClock) clock.Timer {
	return &timer{
		clock: clock,
	}
}

func (t *timer) GetC() (c <-chan time.Time) {
	if t.cancelFunc != nil {
		c = t.signalC
	}
	return
}

func (t *timer) Reset(d time.Duration) (active bool) {
	now := t.clock.Now()
	triggerTime := now.Add(d)

	// Signal our timerSet callback.
	t.clock.signalTimerSet(d, t)

	// Stop our current polling goroutine, if it's running.
	active = t.Stop()

	// Set timer properties.
	t.signalC = make(chan time.Time, 1)
	t.cancelFunc = t.clock.invokeAt(triggerTime, t.signal)
	return
}

func (t *timer) Stop() bool {
	// If the timer is not running, we're done.
	if t.cancelFunc == nil {
		return false
	}

	// Clear our state.
	t.cancelFunc()
	t.cancelFunc = nil
	return true
}

// Sends a single time signal.
func (t *timer) signal(now time.Time) {
	if t.signalC != nil {
		t.signalC <- now
	}
}
