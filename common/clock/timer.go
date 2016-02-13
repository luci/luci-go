// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clock

import (
	"time"
)

// Timer is a wrapper around the time.Timer structure.
//
// A Timer is instantiated from a Clock instance and started via its Reset()
// method.
type Timer interface {
	// GetC returns the underlying timer's channel.
	//
	// If the Timer is interrupted via Stop, its channel will block indefinitely.
	GetC() <-chan TimerResult

	// Reset configures the timer to expire after a specified duration.
	//
	// If the timer is already running, its previous state will be cleared and
	// this method will return true. The channel returned by GetC() will not
	// change due to Reset.
	Reset(d time.Duration) bool

	// Stop clears any timer tasking, rendering it inactive.
	//
	// Stop may be called on an inactive timer, in which case nothing will happen
	// If the timer is active, it will be stopped and this method will return
	// true.
	//
	// If a timer is stopped, its GetC channel will block indefinitely to avoid
	// erroneously unblocking goroutines that are waiting on it. This is
	// consistent with time.Timer.
	Stop() bool
}

// TimerResult is the result for a timer operation.
//
// Time will be set to the time when the result was generated. If the source of
// the result was prematurely terminated (e.g., due to Context cancellation)
// Err will be non-nil.
type TimerResult struct {
	time.Time

	// Err, if not nil, indicates that After did not finish naturally and contains
	// the reason why.
	Err error
}
