// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package common

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func ExampleNewGoroutinePool() {
	pool := NewGoroutinePool(2, NewCanceler())
	for _, s := range []string{"knock!", "knock!"} {
		s := s // Create a new s for closure.
		pool.Schedule(func() { fmt.Print(s) }, nil)
	}
	if pool.Wait() == nil {
		fmt.Printf("\n")
	}
	pool.Cancel(errors.New("pool is no more"))
	pool.Schedule(func() {}, func() {
		fmt.Printf("canceled because %s\n", pool.CancelationReason())
	})
	err := pool.Wait()
	fmt.Printf("all jobs either executed or canceled (%s)\n", err)
	// Output:
	// knock!knock!
	// canceled because pool is no more
	// all jobs either executed or canceled (pool is no more)
}

type newPoolFunc func(p int) GoroutinePool

func TestGoroutinePool(t *testing.T) {
	testGoroutinePool(t, func(p int) GoroutinePool {
		return NewGoroutinePool(p, NewCanceler())
	})
}

func testGoroutinePool(t *testing.T, newPool newPoolFunc) {
	t.Parallel()
	Convey(`A goroutine pool should execute tasks.`, t, func() {

		const MAX = 10
		const J = 200
		pool := newPool(MAX)
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
	testGoroutinePoolCancel(t, func(p int) GoroutinePool {
		return NewGoroutinePool(p, NewCanceler())
	})
}

func testGoroutinePoolCancel(t *testing.T, newPool newPoolFunc) {
	t.Parallel()
	Convey(`A goroutine pool should handle a cancel request.`, t, func() {

		cancelError := errors.New("cancelError")
		const MAX = 10
		const J = 11 * MAX
		pool := newPool(MAX)
		logs := make(chan int, 2*J) // Avoid job blocking when writing to log.
		for i := 1; i <= J; i++ {
			i := i
			pool.Schedule(func() {
				if i == 1 {
					pool.Cancel(cancelError)
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
		So(wait1, ShouldResemble, cancelError)
		So(wait2, ShouldResemble, cancelError)
	})
}

func TestGoroutinePoolCancelFuncCalled(t *testing.T) {
	testGoroutinePoolCancelFuncCalled(t, func(p int) GoroutinePool {
		return NewGoroutinePool(p, NewCanceler())
	})
}

func testGoroutinePoolCancelFuncCalled(t *testing.T, newPool newPoolFunc) {
	t.Parallel()
	Convey(`A goroutine pool should handle an onCancel call.`, t, func() {

		cancelError := errors.New("cancelError")
		// Simulate deterministically when the semaphore returs immediately
		// because of cancelation, as opposed to actually having a resource.
		pipe := make(chan string, 2)
		logs := make(chan string, 3)
		slow := func() {
			// Get 2 item to make sure GoroutinePool can't schedule new job.
			item := <-pipe
			logs <- item
		}
		pool := newPool(1)
		pool.Schedule(slow, slow)
		// This job would have to wait for slow to finish,
		// but slow is waiting for channel to have something.
		// This is sort of a deadlock.
		pool.Schedule(func() { pipe <- "job" }, func() { pipe <- "onCancel" })
		pool.Cancel(cancelError)
		// Canceling should result in onCancel of last job.
		// In case it's a bug, don't wait forever for slow, but unblock slow().
		pipe <- "unblock"
		So(pool.Wait(), ShouldResemble, cancelError)
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
	GoroutinePriorityPool
}

func (c *goroutinePriorityPoolforTest) Schedule(job func(), onCanceled func()) {
	c.GoroutinePriorityPool.Schedule(0, job, onCanceled)
}

func TestGoroutinePriorityPool(t *testing.T) {
	testGoroutinePool(t, func(p int) GoroutinePool {
		return &goroutinePriorityPoolforTest{NewGoroutinePriorityPool(p, NewCanceler())}
	})
}

func TestGoroutinePriorityPoolCancel(t *testing.T) {
	testGoroutinePoolCancel(t, func(p int) GoroutinePool {
		return &goroutinePriorityPoolforTest{NewGoroutinePriorityPool(p, NewCanceler())}
	})
}

func TestGoroutinePriorityPoolCancelFuncCalled(t *testing.T) {
	testGoroutinePoolCancelFuncCalled(t, func(p int) GoroutinePool {
		return &goroutinePriorityPoolforTest{NewGoroutinePriorityPool(p, NewCanceler())}
	})
}

func TestGoroutinePriorityPoolWithPriority(t *testing.T) {
	t.Parallel()
	Convey(`A goroutine pool should execute high priority jobs first.`, t, func(ctx C) {

		const maxPriorities = 15
		pool := NewGoroutinePriorityPool(1, NewCanceler())
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

func assertClosed(t *testing.T, c *canceler) {
	Convey(`A goroutine pool should close channels when cleaning up`, func() {
		// Both channels are unbuffered, so there should be at most one value.
		<-c.channel
		select {
		case _, ok := <-c.channel:
			So(ok, ShouldBeFalse)
		default:
			t.Fatalf("channel is not closed")
		}

		<-c.closeChannel
		select {
		case _, ok := <-c.closeChannel:
			So(ok, ShouldBeFalse)
		default:
			t.Fatalf("channel is not closed")
		}
	})
}

func TestCancelable(t *testing.T) {
	t.Parallel()
	Convey(`A goroutine canceler clean up after a cancel.`, t, func(ctx C) {
		c := newCanceler()
		So(c.CancelationReason(), ShouldBeNil)
		select {
		case <-c.Channel():
			t.FailNow()
		default:
		}

		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case err, isCanceled := <-c.Channel():
				ctx.So(isCanceled, ShouldBeTrue)
				ctx.So(err, ShouldResemble, ErrCanceled)
			}
		}()
		c.Cancel(nil)
		So(c.CancelationReason(), ShouldResemble, ErrCanceled)
		t.Log("waiting for goroutine above to end.")
		wg.Wait()
		So(c.Close(), ShouldBeNil)
		assertClosed(t, c)
	})
}

func TestCancelableDoubleCancel(t *testing.T) {
	t.Parallel()
	Convey(`A goroutine canceler should handle multiple calls.`, t, func() {
		errReason := errors.New("reason")
		errWhatever := errors.New("whatever")
		c := newCanceler()
		c.Cancel(errReason)
		So(c.CancelationReason(), ShouldResemble, errReason)
		c.Cancel(errWhatever)
		So(c.CancelationReason(), ShouldResemble, errReason)
		So(c.Close(), ShouldBeNil)
	})
}

func TestCancelableDoubleCloseAndCancel(t *testing.T) {
	t.Parallel()
	Convey(`A goroutine canceler should handle multiple closes and cancels.`, t, func() {
		errReason := errors.New("reason")
		c := newCanceler()
		So(c.Close(), ShouldBeNil)
		So(c.Close(), ShouldBeNil)
		c.Cancel(errReason)
		So(c.CancelationReason(), ShouldResemble, errReason)
	})
}

func TestCancelableUnblockAfterClosed(t *testing.T) {
	t.Parallel()
	Convey(`A goroutine canceler should clean up when closed.`, t, func(ctx C) {
		c := newCanceler()
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case _, stillOpen := <-c.Channel():
				ctx.So(stillOpen, ShouldBeFalse)
			}
		}()
		So(c.Close(), ShouldBeNil)
		t.Log("waiting for goroutine above to end.")
		wg.Wait()
		assertClosed(t, c)
	})
}
