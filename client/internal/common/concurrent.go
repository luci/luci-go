// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"errors"
	"sync"

	"github.com/maruel/interrupt"
)

// ErrCanceled is the default reason (error) for cancelation of Cancelable.
var ErrCanceled = errors.New("canceled")

// InterruptCanceler embeds maruel/interrupt package in Canceler interface.
var InterruptCanceler Canceler

func init() {
	// TODO(tandrii): maybe move all necessary code from maruel/interrupt here?
	c := newCanceler()
	go func() {
		select {
		case <-interrupt.Channel:
			InterruptCanceler.Cancel(interrupt.ErrInterrupted)
		case <-c.closeChannel:
		}
	}()
	InterruptCanceler = c
}

// TODO(tandrii): Make this fully private once the archiver is migrated to GoroutinePool.

// Semaphore is a classic semaphore with ability to cancel waiting.
type Semaphore interface {
	// Wait waits for the Semaphore to have 1 item available.
	//
	// Returns interrupt.ErrInterrupted if interrupt was triggered.
	// It may return nil even if the interrupt signal is set.
	Wait() error
	// Signal adds 1 to the semaphore.
	Signal()
}

type semaphore chan bool

// NewSemaphore returns a Semaphore of specified size.
func NewSemaphore(size int) Semaphore {
	return newSemaphore(size)
}

func newSemaphore(size int) semaphore {
	s := make(semaphore, size)
	for i := 0; i < cap(s); i++ {
		s <- true
	}
	return s
}

// TODO(tandrii): refactor to use Canceler once Semaphore is not public any more.
func (s semaphore) Wait() error {
	// "select" randomly selects a channel when both channels are available. It
	// is still possible that the other channel is also available.
	select {
	case <-interrupt.Channel:
		return interrupt.ErrInterrupted
	case <-s:
		return nil
	}
}

func (s semaphore) cWait(canceler Canceler) error {
	// "select" randomly selects a channel when both channels are available. It
	// is still possible that the other channel is also available.
	select {
	case <-canceler.Channel():
		return ErrCanceled
	case <-s:
		return nil
	}
}

func (s semaphore) Signal() {
	s <- true
}

// Cancelable implements asynchronous cancelation of execution.
//
// Typical implementation of Cancelable is a pipeline step doing work
// asynchronously in some goroutine. Calling Cancel would ask this goroutine
// to stop processing as soon as possible.
type Cancelable interface {
	// Cancel asks to cancel execution asynchronously.
	//
	// If reason is nil, default ErrCanceled is used.
	// Calling more than once is a no-op: the call is ingored.
	//
	// Cancel opportunistically asks to stop execution, but does not wait
	// for execution to actually stop. It is thus suitable to be called from
	// any other goroutine and will not cause a deadlock.
	Cancel(reason error)

	// CancelationReason returns the reason provided by the first Cancel() call,
	// or nil if not yet canceled.
	CancelationReason() error
}

// Canceler implements Cancelable, and assists in graceful interrupt and
// error handling in pipelines with channels.
type Canceler interface {
	Cancelable

	// Channel returns the channel producing reason values once Cancel() is
	// called. When Close() is called, this channel is closed.
	//
	// Use it in producers to ublock when Cancel() is called:
	//	func produce(out chan <- interface{}, item interface{}) error {
	//		select {
	//			case reason, ok := <- Canceler.Channel():
	//				if !ok {
	//					// Canceler is closed; for us it's just as Canceled.
	//					return ErrCanceled
	//				}
	//				return reason // Canceled with this reason.
	//			case out := <- item:
	//				return nil
	//		}
	//	}
	Channel() <-chan error

	// Close releases all resources and makes the instance unusable.
	// Returns error to implement io.Closer , but it is always nil.
	// Calling more than once is a no-op: the call does nothing.
	Close() error
}

// newInterruptibleCanceler returns a new instance of canceler,
// which cancels itself automatically if InterruptCanceler is cancelled.
func newInterruptibleCanceler() *canceler {
	c := newCanceler()
	go func() {
		select {
		case err, isCanceled := <-InterruptCanceler.Channel():
			if !isCanceled {
				err = errors.New("InterruptCanceler is closed")
			}
			c.Cancel(err)
		case <-c.closeChannel:
		}
	}()
	return c
}

// NewCanceler returns a new instance of canceler.
//
// Call Cancel() once no longer used to release resources.
func NewCanceler() Canceler {
	return newCanceler()
}

// GoroutinePool executes at a limited number of jobs concurrently, queueing
// others.
//
// GoroutinePool implements Canceler allowing canceling all not yet started
// jobs.
type GoroutinePool interface {
	Canceler

	// Wait blocks until all started jobs finish.
	//
	// Returns nil if all jobs have been executed, or reason for cancelation of
	// some jobs. This call is not itself cancelable, meaning that it'll block
	// until all worker goroutines are finished.
	Wait() error
	// Schedule adds a new job for execution as a separate goroutine.
	Schedule(job func())
}

// NewGoroutinePool creates a new GoroutinePool with at most maxConcurrentJobs.
//
// If canceler is not provided, creates a new one.
// GoroutinePool will not start new jobs if cancelled.
//
// Example usage:
//	limiter := NewGoroutinePool(5, nil)
//	for i := range jobs {
//		i := i // Create a new i for closure.
//		limiter.add(func(){work(i)})
//	}
//	if err := limiter.Wait(); err != nil {
//		// Some jobs weren't executed, due to interrupt.
//	} else {
//		// All jobs done.
//	}
func NewGoroutinePool(maxConcurrentJobs int, canceler Canceler) GoroutinePool {
	if canceler == nil {
		canceler = NewCanceler()
	}
	return &goroutinePool{
		Canceler: canceler,
		sema:     newSemaphore(maxConcurrentJobs),
	}
}

type goroutinePool struct {
	Canceler
	wg   sync.WaitGroup
	sema semaphore
}

func (c *goroutinePool) Wait() error {
	c.wg.Wait()
	return c.CancelationReason()
}

func (c *goroutinePool) Schedule(job func()) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if c.sema.cWait(c.Canceler) == nil {
			defer c.sema.Signal()
			// Do not start a new job if canceled.
			if c.CancelationReason() == nil {
				job()
			}
		}
	}()
}

// canceler implements Canceler interface.
type canceler struct {
	channel      chan error
	closeChannel chan bool

	lock   sync.Mutex
	reason error
	closed bool
}

func newCanceler() *canceler {
	return &canceler{
		channel:      make(chan error),
		closeChannel: make(chan bool),
	}
}

func (c *canceler) Cancel(reason error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.reason != nil {
		// Called more than once, ignore.
		return
	}
	if reason == nil {
		reason = ErrCanceled
	}
	c.reason = reason
	if c.closed {
		return
	}
	go func() {
		defer close(c.channel)
		for {
			select {
			case <-c.closeChannel:
				return
			case c.channel <- c.reason:
			}
		}
	}()
}

func (c *canceler) CancelationReason() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.reason
}

func (c *canceler) Channel() <-chan error {
	return c.channel
}

func (c *canceler) Close() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.closeChannel)
	// If Cancel() was called before, then its goroutine will close c.channel.
	// Otherwise, we do.
	if c.reason == nil {
		close(c.channel)
	}
	return nil
}
