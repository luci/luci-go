// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundler

import (
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
)

type timeoutCond struct {
	*sync.Cond

	clock clock.Clock
}

func newTimeoutCond(c clock.Clock, l sync.Locker) *timeoutCond {
	return &timeoutCond{
		Cond:  sync.NewCond(l),
		clock: c,
	}
}

func (c *timeoutCond) waitTimeout(d time.Duration) (timeout bool) {
	if d <= 0 {
		// We've already expired, so don't bother waiting.
		return
	}

	// Kick off a timer to Broadcast when it has completed.
	doneC := make(chan struct{})
	finishedC := make(chan bool)
	defer func() {
		close(doneC)
		timeout = <-finishedC
	}()

	go func() {
		timerExpired := false
		defer func() {
			finishedC <- timerExpired
		}()

		t := c.clock.NewTimer()
		defer t.Stop()
		t.Reset(d)

		// Quickly own our lock. This ensures that we have entered our Wait()
		// method, which in turn ensures that our Broadcast will wake it if
		// necessary.
		func() {
			c.L.Lock()
			defer c.L.Unlock()
		}()

		select {
		case <-doneC:
			break

		case <-t.GetC():
			timerExpired = true
			c.Broadcast()
		}
	}()

	// Wait on our Cond. This will be released when either something else
	// Broadcasts. That "something else" may be our timer expiring.
	c.Wait()
	return
}
