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
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBuffer(t *testing.T) {
	t.Parallel()

	ftt.Run(`A task Buffer`, t, func(t *ftt.Test) {
		b := &Buffer{}
		defer func() {
			if b != nil {
				b.Close()
			}
		}()

		t.Run(`With 10 maximum goroutines, will execute at most 10 simultaneous tasks.`, func(t *ftt.Test) {
			const iters = 1000
			b.Maximum = 10

			called := int32(0)
			max := countMaxGoroutines(iters, 10, func(f func() error) {
				b.WorkC() <- WorkItem{F: func() error {
					atomic.AddInt32(&called, 1)
					return f()
				}}
			})

			assert.Loosely(t, max, should.Equal(10))
			assert.Loosely(t, called, should.Equal(iters))
		})

		t.Run(`Will buffer tasks indefinitely.`, func(t *ftt.Test) {
			const iters = 1000
			b.Maximum = 10

			errC := make([]<-chan error, iters)
			unblockC := make(chan struct{})
			for i := range errC {
				i := i

				errC[i] = b.RunOne(func() error {
					<-unblockC
					return numberError(i)
				})
			}

			// All of the tasks are currently dispatched and blocking on unblockC.
			// Unlock them all and read the resulting errors.
			close(unblockC)

			errs := make([]bool, iters)
			for _, c := range errC {
				errs[int((<-c).(numberError))] = true
			}

			tc := 0
			for _, v := range errs {
				if v {
					tc++
				}
			}
			assert.Loosely(t, tc, should.Equal(iters))
		})

		t.Run(`Can buffer tasks faster than they are reaped via Run.`, func(t *ftt.Test) {
			const iters = 1000
			b.Maximum = 10

			errC := b.Run(func(taskC chan<- func() error) {
				for i := 0; i < iters; i++ {
					i := i
					taskC <- func() error {
						return numberError(i)
					}
				}
			})

			// No errors have been reaped yet, so only b.Maximum tasks should have been
			// dispatched, and they will be blocked on sending to errC. Unblock all of
			// the tasks and confirm that error collection works.
			seen := make(map[int]struct{}, iters)
			for err := range errC {
				seen[int(err.(numberError))] = struct{}{}
			}
			assert.Loosely(t, len(seen), should.Equal(iters))
		})

		t.Run(`Has proper task dispatch order`, func(t *ftt.Test) {
			const iters = 10

			// We use a Maximum of 1 to control task execution order. There should be
			// no data races.
			b.Maximum = 1

			workerStarted := make(chan int, iters+3) // tasks push here; buffered to avoid the need to drain.
			wait := make(chan struct{})              // first task will wait until it is closed.
			gen := func(taskC chan<- func() error) {
				// Start with 2 buffered tasks to fill our work channels so our buffer
				// empty order is deterministic.
				for i := -2; i < 0; i++ {
					i := i
					taskC <- func() error {
						workerStarted <- i
						<-wait
						return nil
					}
				}
				// Ensure 1 task has actually started execution. Note that the task is
				// still running because it's blocked on `wait` channel.
				<-workerStarted
				// Add `iters` tasks which should be executed in the right order.
				for i := 0; i < iters; i++ {
					i := i
					taskC <- func() error {
						workerStarted <- i
						return numberError(i)
					}
				}
				// Finally, add 1 more "sentinel" task ...
				taskC <- func() error {
					workerStarted <- -3
					<-wait
					return nil
				}
				// ... at this point since taskC is unbuffered channel,
				// we are certain Buffer has accepted all `iters` tasks.
				// Unblock the first 2 tasks.
				close(wait)
			}

			account := func(errC <-chan error) []int {
				var order []int
				for err := range errC {
					if err != nil {
						order = append(order, int(err.(numberError)))
					}
				}
				return order
			}

			t.Run(`Is FIFO by default.`, func(t *ftt.Test) {
				assert.Loosely(t, account(b.Run(gen)), should.Resemble([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}))
			})

			t.Run(`Will be LIFO if LIFO is set.`, func(t *ftt.Test) {
				b.SetFIFO(false)
				assert.Loosely(t, account(b.Run(gen)), should.Resemble([]int{9, 8, 7, 6, 5, 4, 3, 2, 1, 0}))
			})
		})

		t.Run(`Will finish tasks if closed while some are pending.`, func(t *ftt.Test) {
			const iters = 1000
			b.Maximum = 10

			// Buffer iters tests. Each will block pending a signal. Since
			// iters > b.Maximum, this ensures that some tasks remain in our Buffer
			// which, in turn, ensures that when we Close the Buffer, it will not be
			// empty.
			workCount := int32(0)
			unblockC := make(chan struct{})
			finishedC := make(chan struct{})
			for i := 0; i < iters; i++ {
				b.WorkC() <- WorkItem{F: func() error {
					<-unblockC
					atomic.AddInt32(&workCount, 1)
					return nil
				}, After: func() {
					if finishedC != nil {
						finishedC <- struct{}{}
					}
				}}
			}

			// First stage: fully execute one of the tasks.
			unblockC <- struct{}{}
			<-finishedC

			// Second stage: Close our Buffer, then unblock the remainder. We
			// synchronize this by waiting for Buffer's internal workC to close. This
			// is okay, since we're not sending more work through it.
			go func() {
				<-b.workC

				finishedC = nil
				close(unblockC)
			}()

			b.Close()
			b = nil // So our test doesn't double-close.

			assert.Loosely(t, workCount, should.Equal(iters))
		})
	})
}
