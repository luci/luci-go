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
	// GetC returns the underlying timer's channel, or nil if the timer is no
	// running.
	GetC() <-chan time.Time

	// Reset configures the timer to expire after a specified duration.
	//
	// If the timer is already running, its previous state will be cleared and
	// this method will return true.
	Reset(d time.Duration) bool

	// Stop clears any timer tasking, rendering it inactive.
	//
	// Stop may be called on an inactive timer, in which case nothing will happen
	// If the timer is active, it will be stopped and this method will return true.
	Stop() bool
}
