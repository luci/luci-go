// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package taskqueueClient defines an AppEngine pull task queue client. This
// client requests tasks from an AppEngine pull task queue and executes them in
// parallel. The client will automatically extend the lease of any owned task
// while it is executing.
package taskqueueClient

import (
	"net/http"
	"time"

	"github.com/luci/luci-go/common/clock"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/paniccatcher"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
	"google.golang.org/api/taskqueue/v1beta2"
)

const (
	// DefaultLease is the default amount of time to lease a task for.
	DefaultLease = 60 * time.Second

	// DefaultNoTasksSleep is the default value for the NoTasksSleep field in
	// Options.
	DefaultNoTasksSleep = 5 * time.Second
)

// Options is the set of configuration options for a task queue client.
type Options struct {
	// Project is the project name that owns the queue.
	Project string
	// Queue is the name of the queue to pull from.
	Queue string
	// Tag, if not empty, is the queue tag to request.
	Tag string

	// Client is the HTTP client to use for task queue API calls. This must be
	// authenticated to lease, update, and delete tasks from the named queue.
	Client *http.Client
	// UserAgent, if not empty, is a user agent string to include in client
	// requests.
	UserAgent string

	// Tasks is the maximum number of simultaneous tasks to execute. If <=0, we
	// will execute one task at a time.
	Tasks int

	// Lease is the amount of time that a task should be leased for.
	//
	// If the task is still executing and half of the lease has expired, the lease
	// will be continuously extended by this amount until the task is finished.
	Lease time.Duration

	// NoTasksSleep is the amount of time to sleep in between lease calls which
	// return no tasks, either from errors or from a lack of available tasks.
	//
	// If zero, DefaultNoTasksSleep will be used.
	NoTasksSleep time.Duration

	// (Testing) If not nil, the testing parameters to use.
	t *testingParams
}

// testingParams are testing parameters that are used to instrument various
// tests for determinism.
type testingParams struct {
	// tq, if not nil, is the task queue instance to use.
	tq taskQueue

	// leaseUpdateC, if not nil, will have a Task written to it each time that
	// Task's lease is updated.
	leaseUpdateC chan *Task
	// leaseUpdateFailedC, if not nil, will have a Task written to it if that
	// Task's lease update failed.
	leaseUpdateFailedC chan *Task
}

// RunFunc is a callback that is invoked to process a given leased task.
//
// If RunFunc returns true, the task has completed and will be deleted from the
// task queue. If it returns false, the task failed to complete, and an attempt
// will be made to immediately release the lease (set duration to 0) so that it
// becomes available for dispatch immediately.
//
// For the duration of RunFunc's execution, the task will be leased. If the
// lease is close to expiration, attempts will be made to renew it.
//
// If, for whatever reason, the lease expires (e.g., loss of connectivity with
// the task queue), RunFunc's context will be cancelled. The task should use
// this signal where possible to abort its work, as it may be handed out to
// another worker by the task queue. If the lease expires, the task will NOT be
// deleted regardless of the return value.
type RunFunc func(context.Context, Task) bool

// taskLoop is the stateful task processing loop.
type taskLoop struct {
	*Options

	rf RunFunc
	tq taskQueue
}

// RunTasks starts a task processing loop. This loop will pull tasks from the
// configured task queue and process them until the supplied Context is
// cancelled, then return.
func RunTasks(c context.Context, o Options, rf RunFunc) {
	svc, err := taskqueue.New(o.Client)
	if err != nil {
		// This can only happen if the Client is nil.
		panic(err)
	}
	svc.UserAgent = o.UserAgent

	if o.Tasks <= 0 {
		o.Tasks = 1
	}
	if o.Lease <= 0 {
		o.Lease = DefaultLease
	}
	if o.NoTasksSleep <= 0 {
		o.NoTasksSleep = DefaultNoTasksSleep
	}

	var tq taskQueue
	if o.t != nil && o.t.tq != nil {
		// (Testing) Use the configured queue.
		tq = o.t.tq
	} else {
		tq = &taskQueueImpl{
			Service: svc,
			project: o.Project,
			queue:   o.Queue,
			tag:     o.Tag,
		}
	}

	// Create and execute our task loop. Provide a work pool for task dispatch.
	tl := taskLoop{
		Options: &o,
		rf:      rf,
		tq:      tq,
	}
	parallel.Ignore(parallel.Run(o.Tasks, func(workC chan<- func() error) {
		tl.process(c, workC)
	}))
}

// process is a run loop that repeatedly leases tasks from the remote task
// queue. Each successfully-leased task will be processed in a goroutine. Lease
// attempts will continue to execute while fewer than the maximum number of
// leases are held.
func (tl *taskLoop) process(c context.Context, workC chan<- func() error) {
	// We will track the current number of available task slots so we know how
	// many to request. We start by assuming that the loop owns all of the
	// semaphore resources tracked in "slots". Each time a task is dispatched, the
	// loop passes ownership of that resource to the task, which releases it upon
	// completion by writing to taskDoneC.
	//
	// Synchronously with our dispatch loop, we reap returned tokens, reclaiming
	// ownership and increasing the number of tokens we're willing to hand outto
	// the next round of tasks.
	slots := tl.Tasks
	taskDoneC := make(chan struct{}, slots)

	for {
		// Has our Context been canceled?
		if err := c.Err(); err != nil {
			log.WithError(err).Debugf(c, "Context has been cancelled.")
			return
		}

		// Reap back any available slots (non-blocking). We do this here to maximize
		// the number of tasks that we request next round.
		for reaping := true; reaping; {
			select {
			case <-taskDoneC:
				slots++
			default:
				reaping = false
			}
		}
		if slots == 0 {
			// No slots available! Block until at least one becomes available, or
			// until we are canceled.
			select {
			case <-taskDoneC:
				slots++
			case <-c.Done():
				return
			}
		}

		// We have any available slots, so make a new lease request. If it fails, or
		// if there are no available tasks, sleep and try again.
		tasks, err := tl.tq.lease(c, slots, tl.Lease)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"slots":      slots,
				"leaseTime":  tl.Lease,
			}.Errorf(c, "Failed to lease new tasks.")
			_ = clock.Sleep(c, tl.NoTasksSleep)
			continue
		}

		if len(tasks) > 0 {
			// Dispatch each task to our work pool.
			for _, t := range tasks {
				t := t
				workC <- func() error {
					defer func() {
						taskDoneC <- struct{}{}
					}()
					tl.handleTask(c, t)
					return nil
				}
			}
			slots -= len(tasks)
		} else {
			// We received no new tasks. Sleep and try again.
			log.Debugf(c, "No tasks received; sleeping...")
			_ = clock.Sleep(c, tl.NoTasksSleep)
		}
	}
}

// handleTask handles an individual task. While it is running, the task will
// repeatedly re-lease itself to maintain ownership. If the task returns true,
// the lease will be modified to zero, forcefully releasing the task.
func (tl *taskLoop) handleTask(c context.Context, t *Task) {
	// If our context has already been cancelled, don't bother running the task.
	if err := c.Err(); err != nil {
		return
	}

	// Per-task cancellable Context.
	taskCtx, cancelFunc := context.WithCancel(c)

	// Spawn a monitoring goroutine that will cancel our task's Context if its
	// lease expires.
	leaseExpiredC := make(chan bool)
	go func() {
		leaseTimer, refreshTimer := clock.NewTimer(clock.Tag(c, "lease")), clock.NewTimer(clock.Tag(c, "refresh"))
		defer leaseTimer.Stop()
		defer refreshTimer.Stop()

		// Called to reset the lease and associated timers when it has been
		// updated.
		resetLease := func(t *Task) {
			delta := t.LeaseExpireTime.Sub(clock.Now(c))

			// If we can't renew the lease and we run overtime, it will expire. We
			// will continuously try and renew the lease during our task's execution,
			// but if we can't for whatever reason, we will give up, accept the
			// expiration, and notify our task that it has expired by cancelling its
			// context.
			//
			// If we wait until the full lease period, L, has elapsed, it's likely
			// that the task has been dispatched elsewhere, and there can be a period
			// where they are both executing. Instead, we will respond to our lease's
			// termination slightly before its actual expiration, either (L-5s) or
			// ((7/8) * L), whichever is greater.
			leaseCutoff := (delta / 8)
			if d := (5 * time.Second); d < leaseCutoff {
				leaseCutoff = d
			}

			refreshTimer.Reset(delta / 2)
			leaseTimer.Reset(delta - leaseCutoff)

			tl.signalLeaseUpdated(t)
		}
		resetLease(t)

		expired := false
		defer func() {
			if expired {
				log.Fields{
					"id": t.ID,
				}.Errorf(c, "Lease has expired.")
				cancelFunc()
			}
			leaseExpiredC <- expired
		}()

		for {
			select {
			case <-taskCtx.Done():
				return

			case tr := <-leaseTimer.GetC():
				expired = !tr.Incomplete()
				return

			case <-refreshTimer.GetC():

				// For determinism, check if our lease has expired before attempting to
				// update it.
				select {
				case tr := <-leaseTimer.GetC():
					expired = !tr.Incomplete()
					return

				default:
					break
				}

				// At least half of our lease has expired. Refresh it.
				//
				// We use "taskCtx" here because if our lease expires while we're
				// waiting to update, we want it to cancel the update call.
				it, err := tl.tq.updateLease(taskCtx, t.ID, tl.Lease)
				if err != nil {
					// If we fail to update the lease, we will allow our standard lease
					// timer to complete, since our task can still potentially finish
					// before it's done.
					log.Fields{
						log.ErrorKey: err,
						"id":         t.ID,
						"duration":   tl.Lease,
					}.Warningf(c, "Failed to update lease duration.")
					tl.signalLeaseUpdateFailed(t)
					continue
				}

				log.Fields{
					"id":       it.ID,
					"duration": tl.Lease,
				}.Debugf(c, "Updated lease duration.")
				resetLease(it)
			}
		}
	}()

	// Execute the task handler function.
	completed := tl.runTask(taskCtx, t)

	// Reap our monitor functions.
	cancelFunc()
	expired := <-leaseExpiredC

	// Update the task queue based on the result.
	//
	// Note that we use the original Context, not the canceled one, for obvious
	// reasons.
	deleted := false
	if !expired {
		if completed {
			log.Fields{
				"id": t.ID,
			}.Debugf(c, "Task has completed; deleting the task.")

			if err := tl.tq.deleteTask(c, t.ID); err != nil {
				log.Fields{
					log.ErrorKey: err,
					"id":         t.ID,
				}.Errorf(c, "Failed to delete the task from the queue.")
			} else {
				deleted = true
			}
		} else {
			log.Fields{
				"id": t.ID,
			}.Debugf(c, "Task did not complete; yielding the lease.")

			if _, err := tl.tq.updateLease(c, t.ID, 0); err != nil {
				log.Fields{
					log.ErrorKey: err,
					"id":         t.ID,
				}.Warningf(c, "Failed task's request to cancel lease.")
			}
			tl.signalLeaseUpdated(t)
		}
	}

	log.Fields{
		"id":           t.ID,
		"leaseExpired": expired,
		"completed":    completed,
		"deleted":      deleted,
	}.Debugf(c, "Task completed.")
}

func (tl *taskLoop) runTask(c context.Context, t *Task) (completed bool) {
	// We'll regard a panic as a transient error.
	defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
		log.Fields{
			"reason": p.Reason,
		}.Errorf(c, "Task handler panicked:\n%s", p.Stack)
		completed = false
	})

	completed = tl.rf(c, *t)
	return
}

// Testing function.
func (tl *taskLoop) signalLeaseUpdated(t *Task) {
	if tl.t != nil && tl.t.leaseUpdateC != nil {
		tl.t.leaseUpdateC <- t
	}
}

// Testing function.
func (tl *taskLoop) signalLeaseUpdateFailed(t *Task) {
	if tl.t != nil && tl.t.leaseUpdateFailedC != nil {
		tl.t.leaseUpdateFailedC <- t
	}
}
