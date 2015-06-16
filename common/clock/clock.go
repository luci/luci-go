// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clock

import (
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
	// Sleeps the current goroutine (see time.Sleep)
	Sleep(time.Duration)
	// Creates a new Timer instance, bound to this Clock.
	NewTimer() Timer
	// Waits a duration, then sends the current time over the returned channel.
	After(time.Duration) <-chan time.Time
}
