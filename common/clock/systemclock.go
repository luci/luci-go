// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clock

import (
	"time"

	"golang.org/x/net/context"
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

func (sc systemClock) Sleep(c context.Context, d time.Duration) error {
	ar := <-sc.After(c, d)
	return ar.Err
}

func (systemClock) NewTimer(ctx context.Context) Timer {
	return newSystemTimer(ctx)
}

func (sc systemClock) After(ctx context.Context, d time.Duration) <-chan TimerResult {
	t := sc.NewTimer(ctx)
	t.Reset(d)
	return t.GetC()
}
