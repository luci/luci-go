// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package parallel

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/luci/luci-go/common/errors"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFanOutIn(t *testing.T) {
	t.Parallel()

	Convey(`A FanOutIn call will run as many goroutines as necessary.`, t, func() {
		const iters = 100

		// Track the number of simultaneous goroutines.
		var max int
		err := FanOutIn(countMaxGoroutines(iters, iters, &max))
		So(err, ShouldBeNil)
		So(max, ShouldEqual, iters)
	})

	Convey(`FanOutIn will return a MultiError if its tasks return an error.`, t, func() {
		terr := errors.New("test error")
		const iters = 100

		err := FanOutIn(func(taskC chan<- func() error) {
			for i := 0; i < iters; i++ {
				i := i

				taskC <- func() error {
					if i == (iters - 1) {
						return terr
					}
					return nil
				}
			}
		})
		So(err, ShouldResemble, errors.MultiError{terr})
	})
}

func countMaxGoroutines(iters, reap int, result *int) func(chan<- func() error) {
	return func(taskC chan<- func() error) {
		maxGoroutines := int32(0)
		var goroutinesLock sync.Mutex
		numGoroutines := int32(0)

		runningC := make(chan struct{})
		blockC := make(chan struct{})

		// Dispatch and reap tasks in batches, since we're blocking dispatch.
		for iters > 0 {
			r := reap
			if reap > iters {
				r = iters
			}
			iters -= r

			for i := 0; i < r; i++ {
				taskC <- func() error {
					cur := atomic.AddInt32(&numGoroutines, 1)
					defer atomic.AddInt32(&numGoroutines, -1)

					// Update our maximum goroutines.
					func() {
						goroutinesLock.Lock()
						defer goroutinesLock.Unlock()

						if maxGoroutines < cur {
							maxGoroutines = cur
						}
					}()

					// Signal that we're running, and stay open until we're released.
					runningC <- struct{}{}
					<-blockC
					return nil
				}
			}

			// Make sure all goroutines are running.
			for i := 0; i < r; i++ {
				<-runningC
			}

			// Release goroutines.
			for i := 0; i < r; i++ {
				blockC <- struct{}{}
			}
		}

		// Write out the result.
		*result = int(maxGoroutines)
	}
}
