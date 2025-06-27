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
	"fmt"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func ExampleWorkPool() {
	val := int32(0)
	err := WorkPool(16, func(workC chan<- func() error) {
		for range 256 {
			workC <- func() error {
				atomic.AddInt32(&val, 1)
				return nil
			}
		}
	})

	if err != nil {
		fmt.Printf("Unexpected error: %s", err.Error())
	}

	fmt.Printf("got: %d", val)
	// Output: got: 256
}

func TestWorkPool(t *testing.T) {
	t.Parallel()

	ftt.Run("When running WorkPool tests", t, func(t *ftt.Test) {
		t.Run("Various sized workpools execute their work successfully", func(t *ftt.Test) {
			val := int32(0)

			t.Run("single goroutine", func(t *ftt.Test) {
				WorkPool(1, func(ch chan<- func() error) {
					for range 100 {
						ch <- func() error { atomic.AddInt32(&val, 1); return nil }
					}
				})

				assert.Loosely(t, val, should.Equal(100))
			})

			t.Run("multiple goroutines", func(t *ftt.Test) {
				WorkPool(10, func(ch chan<- func() error) {
					for range 100 {
						ch <- func() error { atomic.AddInt32(&val, 1); return nil }
					}
				})

				assert.Loosely(t, val, should.Equal(100))
			})

			t.Run("more goroutines than jobs", func(t *ftt.Test) {
				const workers = 10

				// Execute (100*workers) tasks and confirm that only (workers) workers
				// were spawned to handle them.
				var max int
				err := WorkPool(workers, func(taskC chan<- func() error) {
					max = countMaxGoroutines(100*workers, workers, func(f func() error) {
						taskC <- f
					})
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, max, should.Equal(workers))
			})
		})

		t.Run(`<= 0 workers will behave like FanOutIn.`, func(t *ftt.Test) {
			const iters = 100

			// Track the number of simultaneous goroutines.
			var max int
			err := WorkPool(0, func(taskC chan<- func() error) {
				max = countMaxGoroutines(iters, iters, func(f func() error) {
					taskC <- f
				})
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, max, should.Equal(iters))
		})

		t.Run("and testing error handling with a workpool size of 1", func(t *ftt.Test) {
			e1 := errors.New("red fish")
			e2 := errors.New("blue fish")
			t.Run("every job failing returns every error", func(t *ftt.Test) {
				result := WorkPool(1, func(ch chan<- func() error) {
					ch <- func() error { return e1 }
					ch <- func() error { return e2 }
				})

				assert.Loosely(t, result, should.HaveLength(2))
				assert.Loosely(t, result, should.Contain(e1))
				assert.Loosely(t, result, should.Contain(e2))
			})

			t.Run("some jobs failing return those errors", func(t *ftt.Test) {
				result := WorkPool(1, func(ch chan<- func() error) {
					ch <- func() error { return nil }
					ch <- func() error { return e1 }
					ch <- func() error { return nil }
					ch <- func() error { return e2 }
				})

				assert.Loosely(t, result, should.HaveLength(2))
				assert.Loosely(t, result, should.Contain(e1))
				assert.Loosely(t, result, should.Contain(e2))
			})
		})

		t.Run("and testing the worker number parameter", func(t *ftt.Test) {
			started := make([]bool, 2)
			okToTest := make(chan struct{}, 1)
			gogo := make(chan int)
			quitting := make(chan struct{}, 1)

			e1 := errors.New("1 fish")
			e2 := errors.New("2 fish")

			t.Run("2 jobs with 1 worker sequences correctly", func(c *ftt.Test) {
				err := WorkPool(1, func(ch chan<- func() error) {
					ch <- func() error {
						started[0] = true
						okToTest <- struct{}{}
						gogo <- 1
						quitting <- struct{}{}
						return e1
					}
					ch <- func() error {
						started[1] = true
						okToTest <- struct{}{}
						gogo <- 2
						return e2
					}

					<-okToTest
					assert.Loosely(c, started[0], should.BeTrue)
					// Only 1 worker, so the second function should not have started
					// yet.
					assert.Loosely(c, started[1], should.BeFalse)

					assert.Loosely(c, <-gogo, should.Equal(1))
					<-quitting

					// First worker should have died.
					<-okToTest
					assert.Loosely(c, started[1], should.BeTrue)
					assert.Loosely(c, <-gogo, should.Equal(2))
				})
				assert.Loosely(c, err, should.ErrLike(errors.MultiError{
					// Make sure they return in the right order.
					e1, e2,
				}))
			})
		})
	})
}
