// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package taskqueue

import (
	"time"

	"github.com/luci/luci-go/common/errors"

	"golang.org/x/net/context"
)

// Add adds the specified task(s) to the specified task queue.
//
// If only one task is provided its error will be returned directly. If more
// than one task is provided, an errors.MultiError will be returned in the
// event of an error, with a given error index corresponding to the error
// encountered when processing the task at that index.
func Add(c context.Context, queueName string, tasks ...*Task) error {
	lme := errors.NewLazyMultiError(len(tasks))
	i := 0
	err := Raw(c).AddMulti(tasks, queueName, func(t *Task, err error) {
		if !lme.Assign(i, err) {
			*tasks[i] = *t
		}
		i++
	})
	if err == nil {
		err = lme.Get()
		if len(tasks) == 1 {
			err = errors.SingleError(err)
		}
	}
	return err
}

// Delete deletes a task from the task queue.
//
// If only one task is provided its error will be returned directly. If more
// than one task is provided, an errors.MultiError will be returned in the
// event of an error, with a given error index corresponding to the error
// encountered when processing the task at that index.
func Delete(c context.Context, queueName string, tasks ...*Task) error {
	lme := errors.NewLazyMultiError(len(tasks))
	i := 0
	err := Raw(c).DeleteMulti(tasks, queueName, func(err error) {
		lme.Assign(i, err)
		i++
	})
	if err == nil {
		err = lme.Get()
		if len(tasks) == 1 {
			err = errors.SingleError(err)
		}
	}
	return err
}

// NOTE(riannucci): Pull task queues API can be extended to support automatic
// lease management.
//
// The theory is that a good lease API might look like:
//
//   func Lease(queueName, tag string, batchSize int, duration time.Time, cb func(*Task, error<-))
//
// Which blocks and calls cb for each task obtained. Lease would then do all
// necessary backoff negotiation with the backend. The callback could execute
// synchronously (stuffing an error into the chan or panicing if it fails), or
// asynchronously (dispatching a goroutine which will then populate the error
// channel if needed). If it operates asynchronously, it has the option of
// processing multiple work items at a time.
//
// Lease would also take care of calling ModifyLease as necessary to ensure
// that each call to cb would have 'duration' amount of time to work on the
// task, as well as releasing as many leased tasks as it can on a failure.

// Lease leases tasks from a queue.
//
// leaseTime has seconds precision. The number of tasks fetched will be at most
// maxTasks.
func Lease(c context.Context, maxTasks int, queueName string, leaseTime time.Duration) ([]*Task, error) {
	return Raw(c).Lease(maxTasks, queueName, leaseTime)
}

// LeaseByTag leases tasks from a queue, grouped by tag.
//
// If tag is empty, then the returned tasks are grouped by the tag of the task
// with earliest ETA.
//
// leaseTime has seconds precision. The number of tasks fetched will be at most
// maxTasks.
func LeaseByTag(c context.Context, maxTasks int, queueName string, leaseTime time.Duration, tag string) ([]*Task, error) {
	return Raw(c).LeaseByTag(maxTasks, queueName, leaseTime, tag)
}

// ModifyLease modifies the lease of a task.
//
// Used to request more processing time, or to abandon processing. leaseTime has
// seconds precision and must not be negative.
func ModifyLease(c context.Context, task *Task, queueName string, leaseTime time.Duration) error {
	return Raw(c).ModifyLease(task, queueName, leaseTime)
}

// Purge purges all tasks form the named queue.
func Purge(c context.Context, queueName string) error {
	return Raw(c).Purge(queueName)
}

// Stats returns Statistics instances for each of the named task queues.
//
// If only one task is provided its error will be returned directly. If more
// than one task is provided, an errors.MultiError will be returned in the
// event of an error, with a given error index corresponding to the error
// encountered when processing the task at that index.
func Stats(c context.Context, queueNames ...string) ([]Statistics, error) {
	ret := make([]Statistics, len(queueNames))
	lme := errors.NewLazyMultiError(len(queueNames))
	i := 0
	err := Raw(c).Stats(queueNames, func(s *Statistics, err error) {
		if !lme.Assign(i, err) {
			ret[i] = *s
		}
		i++
	})
	if err == nil {
		err = lme.Get()
		if len(queueNames) == 1 {
			err = errors.SingleError(err)
		}
	}
	return ret, err
}

// GetTestable returns a Testable for the current task queue service in c, or
// nil if it does not offer one.
func GetTestable(c context.Context) Testable {
	return Raw(c).GetTestable()
}
