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

	// Wait on our Cond. This will be released when something else Broadcasts.
	waitFinishedC := make(chan struct{})
	defer func() {
		// Cleanup our goroutine prior to exiting.
		<-waitFinishedC
	}()
	go func() {
		defer close(waitFinishedC)
		c.Wait()
	}()

	// Start a timer.
	t := c.clock.NewTimer()
	defer t.Stop()
	t.Reset(d)

	select {
	case <-waitFinishedC:
		return false

	case <-t.GetC():
		// Wake our "wait" goroutine. This will claim the lock, which will put us
		// in the expected state upon return.
		c.Broadcast()
		return true
	}
}
