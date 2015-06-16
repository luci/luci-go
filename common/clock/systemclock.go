// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clock

import (
	"time"
)

// Implementation of Clock that uses Go's standard library.
type systemClock struct{}

// System clock instance.
var systemClockInstance systemClock

var _ Clock = systemClock{}

// GetSystemClock returns an instance of a Clock whose method calls directly use
// Go's "time" library.
func GetSystemClock() Clock {
	return systemClockInstance
}

func (systemClock) Now() time.Time {
	return time.Now()
}

func (systemClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (systemClock) NewTimer() Timer {
	return new(systemTimer)
}

func (systemClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
