// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testclock

import (
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
	"golang.org/x/net/context"
)

// timer is an implementation of clock.TestTimer that uses a channel
// to signal the timer to fire.
//
// The channel is buffered so it can be used without requiring a separate
// signalling goroutine.
type timer struct {
	ctx   context.Context
	clock *testClock

	// tags is the set of tags in the Context when this timer was created.
	tags []string

	// afterC will have the TimerResult of the timer's expiration written to it
	// when this timer triggers or is canceled.
	afterC chan clock.TimerResult

	// cancelMu protects cancelFunc.
	cancelMu sync.Mutex
	// Cancels callback from clock.invokeAt. Being non-nil implies that the timer
	// is active.
	cancelFunc context.CancelFunc
}

var _ clock.Timer = (*timer)(nil)

// NewTimer returns a new, instantiated timer.
func newTimer(ctx context.Context, clk *testClock) *timer {
	return &timer{
		ctx:    ctx,
		clock:  clk,
		tags:   clock.Tags(ctx),
		afterC: make(chan clock.TimerResult, 1),
	}
}

func (t *timer) GetC() <-chan clock.TimerResult {
	return t.afterC
}

func (t *timer) Reset(d time.Duration) (active bool) {
	if d < 0 {
		d = 0
	}

	// Stop our current polling goroutine, if it's running.
	active = t.Stop()

	now := t.clock.Now()
	triggerTime := now.Add(d)

	// Signal our timerSet callback.
	t.clock.signalTimerSet(d, t)

	// Set timer properties.
	t.cancelMu.Lock()
	defer t.cancelMu.Unlock()

	t.cancelFunc = t.clock.invokeAt(t.ctx, triggerTime, func(tr clock.TimerResult) {
		// If our timer is still running, stop it and signal our timer channel.
		if t.Stop() {
			t.afterC <- tr
		}
	})
	return
}

func (t *timer) Stop() (stopped bool) {
	t.cancelMu.Lock()
	defer t.cancelMu.Unlock()

	// If the timer is not running, we're done.
	if t.cancelFunc != nil {
		t.cancelFunc()
		t.cancelFunc = nil
		return true
	}
	return false
}

// GetTags returns the tags associated with the specified timer. If the timer
// has no tags, an empty slice (nil) will be returned.
func GetTags(t clock.Timer) []string {
	if tt, ok := t.(*timer); ok {
		return clock.Tags(tt.ctx)
	}
	return nil
}

// HasTags tests if a given timer has the same tags.
func HasTags(t clock.Timer, first string, tags ...string) bool {
	timerTags := GetTags(t)
	if len(timerTags) == 0 {
		return false
	}

	if first != timerTags[0] {
		return false
	}

	if len(timerTags) > 1 {
		// Compare the remainder.
		timerTags = timerTags[1:]
		if len(timerTags) != len(tags) {
			return false
		}

		for i, tag := range timerTags {
			if tags[i] != tag {
				return false
			}
		}
	}
	return true
}
