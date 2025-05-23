// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parallel

import (
	"sync"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFanOutIn(t *testing.T) {
	t.Parallel()

	ftt.Run(`A FanOutIn call will run as many goroutines as necessary.`, t, func(t *ftt.Test) {
		const iters = 100

		// Track the number of simultaneous goroutines.
		var max int
		err := FanOutIn(func(taskC chan<- func() error) {
			max = countMaxGoroutines(iters, iters, func(f func() error) {
				taskC <- f
			})
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, max, should.Equal(iters))
	})

	ftt.Run(`FanOutIn will return a MultiError if its tasks return an error.`, t, func(t *ftt.Test) {
		terr := errors.New("test error")
		const iters = 100

		err := FanOutIn(func(taskC chan<- func() error) {
			for i := 0; i < iters; i++ {
				taskC <- func() error {
					if i == (iters - 1) {
						return terr
					}
					return nil
				}
			}
		})
		assert.Loosely(t, err, should.ErrLike(errors.MultiError{terr}))
	})
}

func countMaxGoroutines(iters, reap int, enc func(func() error)) int {
	maxGoroutines := 0
	var goroutinesLock sync.Mutex
	numGoroutines := int32(0)

	runningC := make(chan struct{})
	blockC := make(chan struct{})

	// Dispatch and reap tasks in batches, since we're blocking dispatch.
	for iters > 0 {
		r := reap
		if r > iters {
			r = iters
		}
		iters -= r

		for i := 0; i < r; i++ {
			enc(func() error {
				cur := int(atomic.AddInt32(&numGoroutines, 1))
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
			})
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

	return maxGoroutines
}
