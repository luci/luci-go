// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package taskqueue

import "time"

// RawCB is a simple callback for RawInterface.DeleteMulti, getting the error
// for the attempted deletion.
type RawCB func(error)

// RawTaskCB is the callback for RawInterface.AddMulti, getting the added task
// and an error.
type RawTaskCB func(*Task, error)

// RawStatsCB is the callback for RawInterface.Stats. It takes the statistics
// object, as well as an error (e.g. in case the queue doesn't exist).
type RawStatsCB func(*Statistics, error)

// Constraints is the set of implementation-specific task queue constraints.
type Constraints struct {
	// MaxAddSize is the maximum number of tasks that can be added to a queue in
	// a single Add call.
	MaxAddSize int
}

// RawInterface is the full interface to the Task Queue service.
type RawInterface interface {
	// AddMulti adds multiple tasks to the given queue, calling cb for each item.
	//
	// The task passed to the callback function will have all the default values
	// filled in, and will have the Name field populated, if the input task was
	// anonymous (e.g. the Name field was blank).
	AddMulti(tasks []*Task, queueName string, cb RawTaskCB) error
	DeleteMulti(tasks []*Task, queueName string, cb RawCB) error

	Lease(maxTasks int, queueName string, leaseTime time.Duration) ([]*Task, error)
	LeaseByTag(maxTasks int, queueName string, leaseTime time.Duration, tag string) ([]*Task, error)
	ModifyLease(task *Task, queueName string, leaseTime time.Duration) error

	Purge(queueName string) error

	Stats(queueNames []string, cb RawStatsCB) error

	Constraints() Constraints

	GetTestable() Testable
}
