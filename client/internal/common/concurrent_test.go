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

package common

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func ExampleNewGoroutinePool() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := NewGoroutinePool(ctx, 2)
	for _, s := range []string{"knock!", "knock!"} {
		s := s // Create a new s for closure.
		pool.Schedule(func() { fmt.Print(s) }, nil)
	}
	if pool.Wait() == nil {
		fmt.Printf("\n")
	}
	cancel()
	pool.Schedule(func() {}, func() {
		fmt.Printf("canceled because %s\n", ctx.Err())
	})
	err := pool.Wait()
	fmt.Printf("all jobs either executed or canceled (%s)\n", err)
	// Output:
	// knock!knock!
	// canceled because context canceled
	// all jobs either executed or canceled (context canceled)
}

type goroutinePool interface {
	Schedule(job, onCanceled func())
	Wait() error
}

type newPoolFunc func(ctx context.Context, p int) goroutinePool

func TestGoroutinePool(t *testing.T) {
	testGoroutinePool(t, func(ctx context.Context, p int) goroutinePool {
		return NewGoroutinePool(ctx, p)
	})
}

func testGoroutinePool(t *testing.T, newPool newPoolFunc) {
	t.Parallel()
	Convey(`A goroutine pool should execute tasks.`, t, func() {

		const MAX = 10
		const J = 200
		pool := newPool(context.Background(), MAX)
		logs := make(chan int)
		for i := 1; i <= J; i++ {
			i := i
			pool.Schedule(func() {
				logs <- -i
				time.Sleep(time.Millisecond)
				logs <- i
			}, nil)
		}
		var fail error
		go func() {
			defer close(logs)
			fail = pool.Wait()
		}()
		currentJobs := map[int]bool{}
		doneJobs := map[int]bool{}
		for j := range logs {
			if j < 0 {
				t.Logf("Job %d started, %d others running", -j, len(currentJobs))
				So(len(currentJobs) < MAX, ShouldBeTrue)
				currentJobs[-j] = true
			} else {
				t.Logf("Job %d ended", j)
				delete(currentJobs, j)
				doneJobs[j] = true
			}
		}
		So(len(currentJobs), ShouldResemble, 0)
		So(len(doneJobs), ShouldResemble, J)
		So(fail, ShouldBeNil)
	})
}

func TestGoroutinePoolCancel(t *testing.T) {
	testGoroutinePoolCancel(t, func(ctx context.Context, p int) goroutinePool {
		return NewGoroutinePool(ctx, p)
	})
}

func testGoroutinePoolCancel(t *testing.T, newPool newPoolFunc) {
	t.Parallel()
	Convey(`A goroutine pool should handle a cancel request.`, t, func() {
		const MAX = 10
		const J = 11 * MAX
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pool := newPool(ctx, MAX)
		logs := make(chan int, 2*J) // Avoid job blocking when writing to log.
		for i := 1; i <= J; i++ {
			i := i
			pool.Schedule(func() {
				if i == 1 {
					cancel()
				}
				logs <- i
			}, func() {
				// Push negative on canceled tasks.
				logs <- -i
			})
		}
		var wait1 error
		var wait2 error
		go func() {
			defer close(logs)
			wait1 = pool.Wait()
			// Schedule more jobs, all of which must be cancelled.
			for i := J + 1; i <= 2*J; i++ {
				i := i
				pool.Schedule(func() {
					logs <- i
				}, func() {
					logs <- -i
				})
			}
			wait2 = pool.Wait()
		}()
		// At most MAX-1 could have been executing concurrently with job[i=1]
		// after it had cancelled the pool and before it wrote to log.
		// No new job should have started after that. So, log must have at most MAX-1
		// jobs after 1.
		finishedPre := 0
		finishedAfter := 0
		canceled := 0
		for i := range logs {
			if i == 1 {
				finishedAfter++
				break
			}
			finishedPre++
		}
		for i := range logs {
			So(i, ShouldBeLessThanOrEqualTo, J)
			if i > 0 {
				finishedAfter++
			} else {
				canceled++
			}
		}
		t.Logf("JOBS: Total %d Pre %d After %d Cancelled %d", 2*J, finishedPre, finishedAfter, canceled)
		So(finishedPre+finishedAfter+canceled, ShouldResemble, 2*J)
		Convey(fmt.Sprintf("%d (expect < %d MAX) jobs started after cancellation, and at least %d J canceled jobs.", finishedAfter, MAX, J), func() {
			So(finishedAfter, ShouldBeLessThan, MAX)
			So(canceled, ShouldBeGreaterThanOrEqualTo, J)
		})
		So(wait1, ShouldResemble, context.Canceled)
		So(wait2, ShouldResemble, context.Canceled)
	})
}

func TestGoroutinePoolCancelFuncCalled(t *testing.T) {
	testGoroutinePoolCancelFuncCalled(t, func(ctx context.Context, p int) goroutinePool {
		return NewGoroutinePool(ctx, p)
	})
}

func testGoroutinePoolCancelFuncCalled(t *testing.T, newPool newPoolFunc) {
	t.Parallel()
	Convey(`A goroutine pool should handle an onCancel call.`, t, func() {
		// Simulate deterministically when the semaphore returns immediately
		// because of cancellation, as opposed to actually having a resource.
		pipe := make(chan string, 2)
		logs := make(chan string, 3)
		slow := func() {
			// Get 2 item to make sure GoroutinePool can't schedule new job.
			item := <-pipe
			logs <- item
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pool := newPool(ctx, 1)
		pool.Schedule(slow, slow)
		// This job would have to wait for slow to finish,
		// but slow is waiting for channel to have something.
		// This is sort of a deadlock.
		pool.Schedule(func() { pipe <- "job" }, func() { pipe <- "onCancel" })
		cancel()
		// Canceling should result in onCancel of last job.
		// In case it's a bug, don't wait forever for slow, but unblock slow().
		pipe <- "unblock"
		So(pool.Wait(), ShouldResemble, context.Canceled)
		close(pipe)
		if pipeItem, ok := <-pipe; ok {
			logs <- pipeItem
		}
		close(logs)
		for l := range logs {
			if l == "onCancel" {
				return
			}
		}
		t.Fatalf("onCancel wasn't called.")
	})
}

// Purpose: re-use GoroutinePool tests for GoroutinePriorityPool.
type goroutinePriorityPoolforTest struct {
	*GoroutinePriorityPool
}

func (c *goroutinePriorityPoolforTest) Schedule(job func(), onCanceled func()) {
	c.GoroutinePriorityPool.Schedule(0, job, onCanceled)
}

func TestGoroutinePriorityPool(t *testing.T) {
	testGoroutinePool(t, func(ctx context.Context, p int) goroutinePool {
		return &goroutinePriorityPoolforTest{NewGoroutinePriorityPool(ctx, p)}
	})
}

func TestGoroutinePriorityPoolCancel(t *testing.T) {
	testGoroutinePoolCancel(t, func(ctx context.Context, p int) goroutinePool {
		return &goroutinePriorityPoolforTest{NewGoroutinePriorityPool(ctx, p)}
	})
}

func TestGoroutinePriorityPoolCancelFuncCalled(t *testing.T) {
	testGoroutinePoolCancelFuncCalled(t, func(ctx context.Context, p int) goroutinePool {
		return &goroutinePriorityPoolforTest{NewGoroutinePriorityPool(ctx, p)}
	})
}

func TestGoroutinePriorityPoolWithPriority(t *testing.T) {
	t.Parallel()
	Convey(`A goroutine pool should execute high priority jobs first.`, t, func(ctx C) {

		const maxPriorities = 15
		pool := NewGoroutinePriorityPool(context.Background(), 1)
		logs := make(chan int)
		wg := sync.WaitGroup{}
		for i := 0; i < maxPriorities; i++ {
			wg.Add(1)
			i := i
			go func() {
				pool.Schedule(int64(i), func() { logs <- i }, nil)
				wg.Done()
			}()
		}
		wg.Wait()
		var fail error
		go func() {
			defer close(logs)
			fail = pool.Wait()
			ctx.So(fail, ShouldBeNil)
		}()
		doneJobs := make([]bool, maxPriorities)
		// First job can be any, the rest must be in order.
		prio := <-logs
		doneJobs[prio] = true
		for prio := range logs {
			So(doneJobs[prio], ShouldBeFalse)
			doneJobs[prio] = true
			// All higher priority jobs must be finished.
			for before := 0; before < prio; before++ {
				So(doneJobs[before], ShouldBeTrue)
			}
		}
		So(len(doneJobs), ShouldResemble, maxPriorities)
		for _, d := range doneJobs {
			So(d, ShouldBeTrue)
		}
	})
}
