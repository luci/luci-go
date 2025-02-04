// Copyright 2014 The LUCI Authors.
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
	"container/heap"
	"context"
	"slices"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
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

	now time.Time // The current clock time.

	timerCallback TimerCallback // Optional callback when a timer has been set.
	pendingTimers pendingTimerHeap
}

var _ TestClock = (*testClock)(nil)

// New returns a TestClock instance set at the specified time.
func New(now time.Time) TestClock {
	return &testClock{
		now: now,
	}
}

func (c *testClock) Now() time.Time {
	c.Lock()
	defer c.Unlock()

	return c.now
}

func (c *testClock) Sleep(ctx context.Context, d time.Duration) clock.TimerResult {
	t := c.NewTimer(ctx)
	t.Reset(d)
	return <-t.GetC()
}

func (c *testClock) NewTimer(ctx context.Context) clock.Timer {
	t := newTimer(ctx, c)
	return t
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
	triggerTimersLocked(c.now, &c.pendingTimers)
}

func triggerTimersLocked(now time.Time, pendingTimers *pendingTimerHeap) {
	for len(*pendingTimers) > 0 {
		e := (*pendingTimers)[0]
		if now.Before(e.deadline) {
			break
		}

		heap.Pop(pendingTimers)
		e.triggerC <- now
		close(e.triggerC)
	}
}

func (c *testClock) addPendingTimer(t *timer, d time.Duration, triggerC chan<- time.Time) {
	deadline := c.Now().Add(d)
	if callback := c.timerCallback; callback != nil {
		callback(d, t)
	}

	c.Lock()
	defer c.Unlock()

	heap.Push(&c.pendingTimers, &pendingTimer{
		timer:    t,
		deadline: deadline,
		triggerC: triggerC,
	})
	triggerTimersLocked(c.now, &c.pendingTimers)
}

func (c *testClock) clearPendingTimer(t *timer) {
	c.Lock()
	defer c.Unlock()

	c.pendingTimers.cancelAndRemoveTimer(t)
}

func (c *testClock) SetTimerCallback(callback TimerCallback) {
	c.Lock()
	defer c.Unlock()

	c.timerCallback = callback
}

// pendingTimer is a single registered timer instance along with its trigger
// deadline.
type pendingTimer struct {
	*timer

	deadline time.Time
	triggerC chan<- time.Time
}

// pendingTimerHeap is a heap.Interface implementation for a slice of
// pendingTimer.
type pendingTimerHeap []*pendingTimer

func (h pendingTimerHeap) Less(i, j int) bool { return h[i].deadline.Before(h[j].deadline) }
func (h pendingTimerHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h pendingTimerHeap) Len() int           { return len(h) }

func (h *pendingTimerHeap) Push(x any) { *h = append(*h, x.(*pendingTimer)) }
func (h *pendingTimerHeap) Pop() (v any) {
	idx := len(*h) - 1
	v, *h = (*h)[idx], (*h)[:idx]
	return
}

// cancelAndRemoveTimer scans the heap for all *pendingTimers using `t`, closes
// their triggerC, and removes them from the heap.
//
// This preserves the heap invariant.
func (h *pendingTimerHeap) cancelAndRemoveTimer(t *timer) {
	// Remove all pendingTimers which have `t` as their timer.
	*h = slices.DeleteFunc(*h, func(el *pendingTimer) bool {
		if el.timer == t {
			close(el.triggerC)
			return true
		}
		return false
	})

	// Re-heapify the slice.
	heap.Init(h)
}
