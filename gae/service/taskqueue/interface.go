// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueue

// Interface is the full interface to the Task Queue service.
type Interface interface {
	Add(task *Task, queueName string) (*Task, error)
	Delete(task *Task, queueName string) error

	AddMulti(tasks []*Task, queueName string) ([]*Task, error)
	DeleteMulti(tasks []*Task, queueName string) error

	Lease(maxTasks int, queueName string, leaseTime int) ([]*Task, error)
	LeaseByTag(maxTasks int, queueName string, leaseTime int, tag string) ([]*Task, error)
	ModifyLease(task *Task, queueName string, leaseTime int) error

	Purge(queueName string) error

	QueueStats(queueNames []string) ([]Statistics, error)
}
