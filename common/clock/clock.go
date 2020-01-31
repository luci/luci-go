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

// Package clock is an interface to system time and timers which is easy to
// test.
package clock

import (
	"context"
	"time"
)

// Clock is an interface to system time.
//
// The standard clock is SystemClock, which falls through to the system time
// library. Another clock, FakeClock, is available to simulate time facilities
// for testing.
type Clock interface {
	// Returns the current time (see time.Now).
	Now() time.Time

	// Sleeps the current goroutine (see time.Sleep).
	//
	// Sleep will return a TimerResult containing the time when it was awakened
	// and detailing its execution. If the sleep terminated prematurely from
	// cancellation, the TimerResult's Incomplete() method will return true.
	Sleep(context.Context, time.Duration) TimerResult

	// Creates a new Timer instance, bound to this Clock.
	//
	// If the supplied Context is canceled, the timer will expire immediately.
	NewTimer(c context.Context) Timer
}
