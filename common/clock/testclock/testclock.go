// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testclock

import (
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
	"golang.org/x/net/context"
)

// TestClock is a Clock interface with additional methods to help instrument it.
type TestClock interface {
	clock.Clock

	// Set sets the test clock's time.
	Set(time.Time)

	// Add advances the test clock's time.
	Add(time.Duration)

	// SetTimerCallback is a goroutine-safe method to set an instance-wide
	// callback that is invoked when any timer begins.
	SetTimerCallback(TimerCallback)
}

// TimerCallback that can be invoked when a timer has been set. This is useful
// for sychronizing state when testing.
type TimerCallback func(time.Duration, clock.Timer)

// testClock is a test-oriented implementation of the 'Clock' interface.
//
// This implementation's Clock responses are configurable by modifying its
// member variables. Time-based events are explicitly triggered by sending on a
// Timer instance's channel.
type testClock struct {
	sync.Mutex

	now       time.Time  // The current clock time.
	timerCond *sync.Cond // Condition used to manage timer blocking.

	timerCallback TimerCallback // Optional callback when a timer has been set.
}

var _ TestClock = (*testClock)(nil)

// New returns a TestClock instance set at the specified time.
func New(now time.Time) TestClock {
	c := testClock{
		now: now,
	}
	c.timerCond = sync.NewCond(&c)
	return &c
}

func (c *testClock) Now() time.Time {
	c.Lock()
	defer c.Unlock()

	return c.now
}

func (c *testClock) Sleep(ctx context.Context, d time.Duration) error {
	ar := <-c.After(ctx, d)
	return ar.Err
}

func (c *testClock) NewTimer(ctx context.Context) clock.Timer {
	t := newTimer(ctx, c)
	return t
}

func (c *testClock) After(ctx context.Context, d time.Duration) <-chan clock.TimerResult {
	t := newTimer(ctx, c)
	t.Reset(d)
	return t.afterC
}

func (c *testClock) Set(t time.Time) {
	c.Lock()
	defer c.Unlock()

	c.setTimeLocked(t)
}

func (c *testClock) Add(d time.Duration) {
	c.Lock()
	defer c.Unlock()

	c.setTimeLocked(c.now.Add(d))
}

func (c *testClock) setTimeLocked(t time.Time) {
	if t.Before(c.now) {
		panic("Cannot go backwards in time. You're not Doc Brown.")
	}
	c.now = t

	// Unblock any blocking timers that are waiting on our lock.
	c.timerCond.Broadcast()
}

func (c *testClock) SetTimerCallback(callback TimerCallback) {
	c.Lock()
	defer c.Unlock()

	c.timerCallback = callback
}

func (c *testClock) getTimerCallback() TimerCallback {
	c.Lock()
	defer c.Unlock()

	return c.timerCallback
}

func (c *testClock) signalTimerSet(d time.Duration, t clock.Timer) {
	if callback := c.getTimerCallback(); callback != nil {
		callback(d, t)
	}
}

// invokeAt invokes the specified callback when the Clock has advanced at
// or after the specified threshold.
//
// The returned cancelFunc can be used to cancel the blocking. If the cancel
// function is invoked before the callback, the callback will not be invoked.
func (c *testClock) invokeAt(ctx context.Context, threshold time.Time, callback func(clock.TimerResult)) {
	finishedC := make(chan struct{})
	stopC := make(chan struct{})

	// Our control goroutine will monitor both time and stop signals. It will
	// terminate when either the current time has exceeded our time threshold or
	// it has been signalled to terminate via Context.
	//
	// The lock that we take our here is owned by the following goroutine.
	c.Lock()
	go func() {
		defer close(finishedC)

		defer func() {
			now := c.now
			c.Unlock()

			// If we finished naturally, but our Context is Done, include the Context
			// error.
			callback(clock.TimerResult{Time: now, Err: ctx.Err()})
		}()

		for {
			// Determine if we are past our signalling threshold. We can safely access
			// our clock's time member directly, since we hold its lock from the
			// condition firing.
			if !c.now.Before(threshold) {
				return
			}

			// Wait for a signal from our clock's condition.
			c.timerCond.Wait()

			// Have we been stopped?
			select {
			case <-stopC:
				return

			default:
				// Nope.
			}
		}
	}()

	// This goroutine will monitor our Context and unblock if it expires before
	// the designated test time.
	go func() {
		select {
		case <-finishedC:
			// Our timer has expired, so no need to further monitor.
			return

		case <-ctx.Done():
			// If we finish at the same time our Context is canceled, we don't need to
			// wake our monitor (determinism).
			select {
			case <-finishedC:
				return
			default:
				break
			}

			// Our Context has been canceled. Forward this to our monitor goroutine
			// and wake our condition.
			close(stopC)
			c.timerCond.Broadcast()
		}
	}()
}
