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
	"container/heap"
	"context"
	"os"
	"os/signal"
	"sync"
)

type semaphore chan bool

func newSemaphore(size int) semaphore {
	s := make(semaphore, size)
	for i := 0; i < cap(s); i++ {
		s <- true
	}
	return s
}

// wait waits for the Semaphore to have 1 item available.
//
// If nil is returned, the caller must call .signal() afterward.
func (s semaphore) wait(ctx context.Context) error {
	// Always check Err first. "select" randomly selects a channel when both
	// channels are available, and we want to prioritize cancellation.
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		// Err() is guaranteed to be non-nil.
		return ctx.Err()
	case <-s:
		/*
			// Do a last check, just in case. It makes things slightly slower but
			// reduces the chance of race condition with having a spurious item running
			// after cancellation.
			if err := ctx.Err(); err != nil {
				s.signal()
				return err
			}
		*/
		return nil
	}
}

// signal adds 1 to the semaphore.
func (s semaphore) signal() {
	s <- true
}

// CancelOnCtrlC returns a new Context that is canceled on Ctrl-C
// (os.Interrupt).
func CancelOnCtrlC(ctx context.Context) context.Context {
	ctx2, cancel := context.WithCancel(ctx)
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)
	go func() {
		select {
		case <-interrupted:
			cancel()
		case <-ctx2.Done():
		}
		signal.Stop(interrupted)
	}()
	return ctx2
}

// NewGoroutinePool creates a new GoroutinePool running at most
// maxConcurrentJobs concurrent operations.
func NewGoroutinePool(ctx context.Context, maxConcurrentJobs int) *GoroutinePool {
	return &GoroutinePool{
		ctx:  ctx,
		sema: newSemaphore(maxConcurrentJobs),
	}
}

// GoroutinePool executes at a limited number of jobs concurrently, queueing
// others.
type GoroutinePool struct {
	ctx  context.Context
	wg   sync.WaitGroup
	sema semaphore
}

// Wait blocks until all started jobs are done, or the context is canceled.
//
// Returns nil if all jobs have been executed, or the error if the associated
// context is canceled.
func (g *GoroutinePool) Wait() error {
	g.wg.Wait()
	return g.ctx.Err()
}

// Schedule adds a new job for execution as a separate goroutine.
//
// If the GoroutinePool context is canceled, onCanceled is called instead. It
// is fine to pass nil as onCanceled.
func (g *GoroutinePool) Schedule(job, onCanceled func()) {
	g.wg.Add(1)
	// Note: We could save a bit of memory by preallocating maxConcurrentJobs
	// goroutines instead of one goroutine per item.
	go func() {
		defer g.wg.Done()
		if g.sema.wait(g.ctx) == nil {
			defer g.sema.signal()
			job()
		} else if onCanceled != nil {
			onCanceled()
		}
	}()
}

// NewGoroutinePriorityPool creates a new goroutine pool with at most
// maxConcurrentJobs.
//
// Each task is run according to the priority of each item.
func NewGoroutinePriorityPool(ctx context.Context, maxConcurrentJobs int) *GoroutinePriorityPool {
	pool := &GoroutinePriorityPool{
		ctx:   ctx,
		sema:  newSemaphore(maxConcurrentJobs),
		tasks: priorityQueue{},
	}
	return pool
}

// GoroutinePriorityPool executes a limited number of jobs concurrently,
// queueing others.
type GoroutinePriorityPool struct {
	ctx   context.Context
	sema  semaphore
	wg    sync.WaitGroup
	mu    sync.Mutex
	tasks priorityQueue
}

// Wait blocks until all started jobs are done, or the context is canceled.
//
// Returns nil if all jobs have been executed, or the error if the associated
// context is canceled.
func (g *GoroutinePriorityPool) Wait() error {
	g.wg.Wait()
	return g.ctx.Err()
}

// Schedule adds a new job for execution as a separate goroutine.
//
// If the GoroutinePriorityPool is canceled, onCanceled is called instead. It
// is fine to pass nil as onCanceled. Smaller values of priority imply earlier
// execution.
//
// The lower the priority value, the higher the priority of the item.
func (g *GoroutinePriorityPool) Schedule(priority int64, job, onCanceled func()) {
	g.wg.Add(1)
	t := &task{priority, job, onCanceled}
	g.mu.Lock()
	heap.Push(&g.tasks, t)
	g.mu.Unlock()
	go g.executeOne()
}

func (g *GoroutinePriorityPool) executeOne() {
	// Note: We could save a bit of memory by preallocating maxConcurrentJobs
	// goroutines instead of one goroutine per item.
	defer g.wg.Done()
	if g.sema.wait(g.ctx) == nil {
		defer g.sema.signal()
		g.popTask().job()
	} else if task := g.popTask(); task.onCanceled != nil {
		task.onCanceled()
	}
}

func (g *GoroutinePriorityPool) popTask() *task {
	g.mu.Lock()
	t := heap.Pop(&g.tasks).(*task)
	g.mu.Unlock()
	return t
}

type task struct {
	priority   int64
	job        func()
	onCanceled func()
}

// A priorityQueue implements heap.Interface and holds tasks.
type priorityQueue []*task

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].priority < pq[j].priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x interface{}) {
	item := x.(*task)
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	// Downsize the memory used only if it's saving 512 items. It is a trade off
	// between a pure memory leak and heap churn.
	if cap(*pq)-len(*pq) >= 512 {
		new := make(priorityQueue, len(*pq))
		copy(new, *pq)
		*pq = new
	}
	return item
}
