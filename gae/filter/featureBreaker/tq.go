// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package featureBreaker

import (
	"time"

	"golang.org/x/net/context"

	tq "github.com/luci/gae/service/taskqueue"
)

type tqState struct {
	*state

	tq tq.RawInterface
}

var _ tq.RawInterface = (*tqState)(nil)

func (t *tqState) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	return t.run(func() (err error) { return t.tq.AddMulti(tasks, queueName, cb) })
}

func (t *tqState) DeleteMulti(tasks []*tq.Task, queueName string, cb tq.RawCB) error {
	return t.run(func() error { return t.tq.DeleteMulti(tasks, queueName, cb) })
}

func (t *tqState) Lease(maxTasks int, queueName string, leaseTime time.Duration) (tasks []*tq.Task, err error) {
	err = t.run(func() (err error) {
		tasks, err = t.tq.Lease(maxTasks, queueName, leaseTime)
		return
	})
	if err != nil {
		tasks = nil
	}
	return
}

func (t *tqState) LeaseByTag(maxTasks int, queueName string, leaseTime time.Duration, tag string) (tasks []*tq.Task, err error) {
	err = t.run(func() (err error) {
		tasks, err = t.tq.LeaseByTag(maxTasks, queueName, leaseTime, tag)
		return
	})
	if err != nil {
		tasks = nil
	}
	return
}

func (t *tqState) ModifyLease(task *tq.Task, queueName string, leaseTime time.Duration) error {
	return t.run(func() error { return t.tq.ModifyLease(task, queueName, leaseTime) })
}

func (t *tqState) Purge(queueName string) error {
	return t.run(func() error { return t.tq.Purge(queueName) })
}

func (t *tqState) Stats(queueNames []string, cb tq.RawStatsCB) error {
	return t.run(func() error { return t.tq.Stats(queueNames, cb) })
}

func (t *tqState) Constraints() tq.Constraints {
	return t.tq.Constraints()
}

func (t *tqState) GetTestable() tq.Testable {
	return t.tq.GetTestable()
}

// FilterTQ installs a featureBreaker TaskQueue filter in the context.
func FilterTQ(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return tq.AddRawFilters(c, func(ic context.Context, tq tq.RawInterface) tq.RawInterface {
		return &tqState{state, tq}
	}), state
}
