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
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type numberError int32

func (e numberError) Error() string {
	return fmt.Sprintf("#%d", e)
}

func TestRunner(t *testing.T) {
	t.Parallel()

	ftt.Run("When using a Runner directly", t, func(t *ftt.Test) {
		r := &Runner{}
		defer func() {
			if r != nil {
				r.Close()
			}
		}()

		t.Run(`Can schedule individual tasks.`, func(t *ftt.Test) {
			const iters = 100

			ac := int32(0)
			resultC := make(chan int32)

			// Dispatch iters tasks.
			for i := 0; i < iters; i++ {
				i := i

				errC := r.RunOne(func() error {
					atomic.AddInt32(&ac, int32(i))
					return numberError(i)
				})

				// Reap errC.
				go func() {
					resultC <- int32((<-errC).(numberError))
				}()
			}

			// Reap the results and compare.
			result := int32(0)
			for i := 0; i < iters; i++ {
				result += <-resultC
			}
			assert.That(t, result, should.Equal(atomic.LoadInt32(&ac)))
		})

		t.Run(`Can use WorkC directly in a bidirectional select loop.`, func(t *ftt.Test) {
			// Generate a function that writes a value to "outC".
			outC := make(chan int)
			valueWriter := func(v int) func() error {
				return func() error {
					outC <- v
					return nil
				}
			}

			// We will repeatedly dispatch tasks and reap their results in the same
			// select loop.
			//
			// remaining is the total number of tasks to dispatch. dispatch controls
			// the number of simultaneous tasks that we're willing to send at a time.
			// count is the number of tasks that have completed.
			remaining := 1000
			count := 0
			dispatch := 10
			wc := r.WorkC()

			for count < 1000 {
				select {
				case wc <- WorkItem{F: valueWriter(1)}:
					dispatch--
					remaining--

				case v := <-outC:
					count += v
					dispatch++
				}

				if dispatch == 0 || remaining == 0 {
					// Stop writing work items until we reap some results.
					wc = nil
				} else {
					// We have results, start writing work items again.
					wc = r.WorkC()
				}
			}

			assert.Loosely(t, count, should.Equal(1000))
		})

		t.Run(`A WorkItem's After method can recover from a panic.`, func(t *ftt.Test) {
			testErr := errors.New("test error")

			var err error
			r.WorkC() <- WorkItem{
				F: func() error {
					panic(testErr)
				},
				After: func() {
					if r := recover(); r != nil {
						err = r.(error)
					}
				},
			}

			r.Close()
			r = nil // Do we don't close it in a defer.

			assert.Loosely(t, err, should.Equal(testErr))
		})

		t.Run("Ignore consumes the errors and blocks", func(t *ftt.Test) {
			count := new(int32)
			Ignore(r.Run(func(ch chan<- func() error) {
				for i := 0; i < 100; i++ {
					ch <- func() error {
						atomic.AddInt32(count, 1)
						return fmt.Errorf("whaaattt")
					}
				}
			}))
			assert.Loosely(t, *count, should.Equal(100))
		})

		t.Run("Must panics on the first error", func(t *ftt.Test) {
			r.Maximum = 1
			count := new(int32)

			// Reap errC at the end, since Must will panic without consuming its
			// contents.
			var errC <-chan error
			defer func() {
				if errC != nil {
					for range errC {
					}
				}
			}()

			assert.Loosely(t, func() {
				errC = r.Run(func(ch chan<- func() error) {
					for i := 0; i < 100; i++ {
						i := i
						ch <- func() error {
							atomic.AddInt32(count, 1)
							return fmt.Errorf("whaaattt: %d", i)
						}
					}
				})
				Must(errC)
			}, should.PanicLike("whaaattt: 0"))
			// Either:
			//   * the panic happened and we load count before ch is unblocked
			//   * the panic happened then ch(1) pushes and runs, then we load count
			// So count will either be 1 or 2, but never more or less.
			assert.Loosely(t, atomic.LoadInt32(count), should.BeBetweenOrEqual(1, 2))
		})
	})
}
