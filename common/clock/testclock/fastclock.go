// Copyright 2021 The LUCI Authors.
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
	"fmt"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
)

// FastClock mimics faster physical clock in tests.
//
// Its API is exactly the same as that of TestClock and can be used in-place.
// However, unlike TestClock, the time in this clock moves forward as
// a normal wall clock would, but at an arbitrarily faster rate.
//
// It's useful for integration tests simulating a large system over long period
// of (wall clock) time where adjusting each indiviudal timeout/delay/timer via
// testclock API isn't feasible.
type FastClock struct {
	mutex          sync.Mutex
	initSysTime    time.Time
	initFastTime   time.Time
	fastToSysRatio int
	sysTimerSleep  time.Duration
	done           chan struct{}
	pendingTimers  pendingTimerHeap
	timerCallback  TimerCallback
	systemNow      func() time.Time // mocked in tests.
}

// NewFastClock creates a new FastClock running faster than a system clock.
//
// You SHOULD call .Close() on the returned object after use to avoid leaks.
func NewFastClock(now time.Time, ratio int) *FastClock {
	f := &FastClock{
		initFastTime:   now,
		initSysTime:    time.Now(),
		fastToSysRatio: ratio,
		sysTimerSleep:  10 * time.Microsecond,
		done:           make(chan struct{}),
		systemNow:      time.Now,
	}
	go f.periodicTimerNotify()
	return f
}

// periodicTimerNotify follows system (wall) clock and wakes us timers as
// necessary.
func (f *FastClock) periodicTimerNotify() {
	notify := func() {
		f.mutex.Lock()
		defer f.mutex.Unlock()
		triggerTimersLocked(f.Now(), &f.pendingTimers)
	}

	// TODO(tandrii): wait until the earliest timer for efficiency.
	sysTimer := time.NewTimer(f.sysTimerSleep)
	for {
		select {
		case <-f.done:
			return
		case <-sysTimer.C:
			notify()
			sysTimer.Reset(f.sysTimerSleep)
		}
	}
}

// Close frees clock resources.
func (f *FastClock) Close() {
	close(f.done)
}

// Now returns the current time (see time.Now).
func (f *FastClock) Now() time.Time {
	_, fNow := f.now()
	return fNow
}

// now returns system (wall) clock time and this clock's time.
func (f *FastClock) now() (time.Time, time.Time) {
	sNow := f.systemNow()
	fNow := f.initFastTime.Add(sNow.Sub(f.initSysTime) * time.Duration(f.fastToSysRatio))
	return sNow, fNow
}

// Sleep sleeps the current goroutine (see time.Sleep).
//
// Sleep will return a TimerResult containing the time when it was awakened
// and detailing its execution. If the sleep terminated prematurely from
// cancellation, the TimerResult's Incomplete() method will return true.
func (f *FastClock) Sleep(ctx context.Context, d time.Duration) clock.TimerResult {
	t := f.NewTimer(ctx)
	t.Reset(d)
	return <-t.GetC()
}

// NewTimer creates a new Timer instance, bound to this Clock.
//
// If the supplied Context is canceled, the timer will expire immediately.
func (f *FastClock) NewTimer(ctx context.Context) clock.Timer {
	return newTimer(ctx, f)
}

// Set sets the test clock's time to at least the given time.
//
// Noop if Now() is already after the given time.
func (f *FastClock) Set(fNew time.Time) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	sNow, fBefore := f.now()
	if fBefore.After(fNew) {
		// fNew is already in the past.
		return
	}
	f.initSysTime = sNow
	f.initFastTime = fNew

	triggerTimersLocked(fNew, &f.pendingTimers)
}

// Add advances the test clock's time.
func (f *FastClock) Add(d time.Duration) {
	if d < 0 {
		panic(fmt.Errorf("cannot go backwards in time. You're not Doc Brown.\nDelta: %s", d))
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	sNow, fBefore := f.now()
	f.initSysTime = sNow
	f.initFastTime = fBefore.Add(d)
	triggerTimersLocked(f.initFastTime, &f.pendingTimers)
}

// SetTimerCallback is a goroutine-safe method to set an instance-wide
// callback that is invoked when any timer begins.
func (f *FastClock) SetTimerCallback(clbk TimerCallback) {
	f.mutex.Lock()
	f.timerCallback = clbk
	f.mutex.Unlock()
}

func (f *FastClock) addPendingTimer(t *timer, d time.Duration, triggerC chan<- time.Time) {
	deadline := f.Now().Add(d)
	if callback := f.timerCallback; callback != nil {
		callback(d, t)
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	heap.Push(&f.pendingTimers, &pendingTimer{
		timer:    t,
		deadline: deadline,
		triggerC: triggerC,
	})
	_, now := f.now()
	triggerTimersLocked(now, &f.pendingTimers)
}

func (f *FastClock) clearPendingTimer(t *timer) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	for i := 0; i < len(f.pendingTimers); {
		if e := f.pendingTimers[0]; e.timer == t {
			heap.Remove(&f.pendingTimers, i)
			close(e.triggerC)
		} else {
			i++
		}
	}
}
