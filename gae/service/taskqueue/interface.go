// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueue

// Interface is the full interface to the Task Queue service.
type Interface interface {
	// NewTask simply creates a new Task object with the Path field populated.
	// The path parameter may be blank, if you want to use the default task path
	// ("/_ah/queue/<queuename>").
	NewTask(path string) *Task

	Add(task *Task, queueName string) error
	Delete(task *Task, queueName string) error

	AddMulti(tasks []*Task, queueName string) error
	DeleteMulti(tasks []*Task, queueName string) error

	// NOTE(riannucci): No support for pull taskqueues. We're not planning on
	// making pull-queue clients which RUN in appengine (e.g. they'd all be
	// external REST consumers). If someone needs this, it will need to be added
	// here and in RawInterface. The theory is that a good lease API might look
	// like:
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

	Purge(queueName string) error

	Stats(queueNames ...string) ([]Statistics, error)

	Testable() Testable

	Raw() RawInterface
}
