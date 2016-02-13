// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package parallel

// Semaphore is a sync.Locker that implements a n-semaphore.
//
// Lock the semaphore acquires a semaphore token, possibly blocking until one
// is available.
//
// Unlock releases an owned token, returning it to the semaphore.
type Semaphore chan struct{}

// Lock acquires a semaphore resource, blocking until one is available.
func (s Semaphore) Lock() {
	if cap(s) > 0 {
		s <- struct{}{}
	}
}

// Unlock releases a single semaphore resource.
func (s Semaphore) Unlock() {
	if cap(s) > 0 {
		<-s
	}
}

// TakeAll blocks until it holds all available semaphore resources. When it
// returns, the caller owns all of the resources in the semaphore.
func (s Semaphore) TakeAll() {
	for i := 0; i < cap(s); i++ {
		s.Lock()
	}
}
