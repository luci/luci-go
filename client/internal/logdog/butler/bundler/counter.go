// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundler

import (
	"sync/atomic"
)

// counter is a goroutine-safe monotonically-increasing counter.
type counter struct {
	// current is the current counter value.
	//
	// It must be the first field in the struct to ensure it's 64-bit aligned
	// for atomic operations.
	current int64
}

func (c *counter) next() int64 {
	// AddInt64 returns the value + 1, so subtract one to get its previous value
	// (current counter value).
	return atomic.AddInt64(&c.current, 1) - 1
}
