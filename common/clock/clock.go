// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clock

import (
	"time"

	"golang.org/x/net/context"
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
	// If the supplied Context is canceled prior to the specified duration,
	// Sleep will return the Context's error. If the sleep completes naturally,
	// it will return nil.
	Sleep(context.Context, time.Duration) error

	// Creates a new Timer instance, bound to this Clock.
	//
	// If the supplied Context is canceled, the timer will expire immediately.
	NewTimer(c context.Context) Timer

	// Waits a duration, then sends the current time over the returned channel.
	//
	// If the supplied Context is canceled, the timer will expire immediately.
	After(context.Context, time.Duration) <-chan TimerResult
}
