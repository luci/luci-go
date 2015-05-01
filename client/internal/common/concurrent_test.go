// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/maruel/ut"
)

func TestGoroutinePool(t *testing.T) {
	t.Parallel()

	const MAX = 10
	const J = 200
	pool := NewGoroutinePool(MAX, NewCanceler())
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
			ut.AssertEqual(t, true, len(currentJobs) < MAX)
			currentJobs[-j] = true
		} else {
			t.Logf("Job %d ended", j)
			delete(currentJobs, j)
			doneJobs[j] = true
		}
	}
	ut.AssertEqual(t, 0, len(currentJobs))
	ut.AssertEqual(t, J, len(doneJobs))
	ut.AssertEqual(t, nil, fail)
}

func TestGoroutinePoolCancel(t *testing.T) {
	t.Parallel()

	cancelError := errors.New("cancelError")
	const MAX = 10
	const J = 11 * MAX
	pool := NewGoroutinePool(MAX, NewCanceler())
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
	count := 0
	canceled := 0
	for i := range logs {
		if i == 1 {
			for i = range logs {
				ut.AssertEqual(t, true, i <= J)
				if i > 0 {
					count++
				} else {
					canceled++
				}
			}
		}
	}
	ut.AssertEqualf(t, true, count < MAX, "%d (>=%d MAX) jobs started after cancellation.", count, MAX)
	ut.AssertEqualf(t, true, canceled > J, "%d > %d", canceled, J)
	ut.AssertEqual(t, cancelError, wait1)
	ut.AssertEqual(t, cancelError, wait2)
}

func assertClosed(t *testing.T, c *canceler) {
	// Both channels are unbuffered, so there should be at most one value.
	for range c.channel {
		break
	}
	select {
	case _, ok := <-c.channel:
		ut.AssertEqual(t, false, ok)
	default:
		t.Fatalf("channel is not closed")
	}
	for range c.closeChannel {
		break
	}
	select {
	case _, ok := <-c.closeChannel:
		ut.AssertEqual(t, false, ok)
	default:
		t.Fatalf("channel is not closed")
	}
}

func TestCancelable(t *testing.T) {
	t.Parallel()
	c := newCanceler()
	ut.AssertEqual(t, nil, c.CancelationReason())
	select {
	case <-c.Channel():
		t.Fatal()
	default:
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case err, isCanceled := <-c.Channel():
			ut.AssertEqualf(t, true, isCanceled, "Closed, but shouldn't be.")
			ut.AssertEqual(t, ErrCanceled, err)
		}
	}()
	c.Cancel(nil)
	ut.AssertEqual(t, ErrCanceled, c.CancelationReason())
	t.Log("waiting for goroutine above to end.")
	wg.Wait()
	ut.AssertEqual(t, nil, c.Close())
	assertClosed(t, c)
}

func TestCancelableDoubleCancel(t *testing.T) {
	t.Parallel()
	errReason := errors.New("reason")
	errWhatever := errors.New("whatever")
	c := newCanceler()
	c.Cancel(errReason)
	ut.AssertEqual(t, errReason, c.CancelationReason())
	c.Cancel(errWhatever)
	ut.AssertEqual(t, errReason, c.CancelationReason())
	ut.AssertEqual(t, nil, c.Close())
}

func TestCancelableDoubleCloseAndCancel(t *testing.T) {
	t.Parallel()
	errReason := errors.New("reason")
	c := newCanceler()
	ut.AssertEqual(t, nil, c.Close())
	ut.AssertEqual(t, nil, c.Close())
	c.Cancel(errReason)
	ut.AssertEqual(t, errReason, c.CancelationReason())
}

func TestCancelableUnblockAfterClosed(t *testing.T) {
	t.Parallel()
	c := newCanceler()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case _, stillOpen := <-c.Channel():
			ut.AssertEqual(t, false, stillOpen)
		}
	}()
	ut.AssertEqual(t, nil, c.Close())
	t.Log("waiting for goroutine above to end.")
	wg.Wait()
	assertClosed(t, c)
}
