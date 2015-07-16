// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"golang.org/x/net/context"
)

// TaskQueue is the full interface to the Task Queue service.
type TaskQueue interface {
	Add(task *TQTask, queueName string) (*TQTask, error)
	Delete(task *TQTask, queueName string) error

	AddMulti(tasks []*TQTask, queueName string) ([]*TQTask, error)
	DeleteMulti(tasks []*TQTask, queueName string) error

	Lease(maxTasks int, queueName string, leaseTime int) ([]*TQTask, error)
	LeaseByTag(maxTasks int, queueName string, leaseTime int, tag string) ([]*TQTask, error)
	ModifyLease(task *TQTask, queueName string, leaseTime int) error

	Purge(queueName string) error

	QueueStats(queueNames []string) ([]TQStatistics, error)
}

// TQFactory is the function signature for factory methods compatible with
// SetTQFactory.
type TQFactory func(context.Context) TaskQueue

// GetTQ gets the TaskQueue implementation from context.
func GetTQ(c context.Context) TaskQueue {
	if f, ok := c.Value(taskQueueKey).(TQFactory); ok && f != nil {
		return f(c)
	}
	return nil
}

// SetTQFactory sets the function to produce TaskQueue instances, as returned by
// the GetTQ method.
func SetTQFactory(c context.Context, tqf TQFactory) context.Context {
	return context.WithValue(c, taskQueueKey, tqf)
}

// SetTQ sets the current TaskQueue object in the context. Useful for testing
// with a quick mock. This is just a shorthand SetTQFactory invocation to set
// a factory which always returns the same object.
func SetTQ(c context.Context, tq TaskQueue) context.Context {
	return SetTQFactory(c, func(context.Context) TaskQueue { return tq })
}
