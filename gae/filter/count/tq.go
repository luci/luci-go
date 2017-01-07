// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package count

import (
	"time"

	"golang.org/x/net/context"

	tq "github.com/luci/gae/service/taskqueue"
)

// TQCounter is the counter object for the TaskQueue service.
type TQCounter struct {
	AddMulti    Entry
	DeleteMulti Entry
	Lease       Entry
	LeaseByTag  Entry
	ModifyLease Entry
	Purge       Entry
	Stats       Entry
}

type tqCounter struct {
	c *TQCounter

	tq tq.RawInterface
}

var _ tq.RawInterface = (*tqCounter)(nil)

func (t *tqCounter) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	return t.c.AddMulti.up(t.tq.AddMulti(tasks, queueName, cb))
}

func (t *tqCounter) DeleteMulti(tasks []*tq.Task, queueName string, cb tq.RawCB) error {
	return t.c.DeleteMulti.up(t.tq.DeleteMulti(tasks, queueName, cb))
}

func (t *tqCounter) Lease(maxTasks int, queueName string, leaseTime time.Duration) ([]*tq.Task, error) {
	tasks, err := t.tq.Lease(maxTasks, queueName, leaseTime)
	t.c.Lease.up(err)
	return tasks, err
}

func (t *tqCounter) LeaseByTag(maxTasks int, queueName string, leaseTime time.Duration, tag string) ([]*tq.Task, error) {
	tasks, err := t.tq.LeaseByTag(maxTasks, queueName, leaseTime, tag)
	t.c.LeaseByTag.up(err)
	return tasks, err
}

func (t *tqCounter) ModifyLease(task *tq.Task, queueName string, leaseTime time.Duration) error {
	return t.c.ModifyLease.up(t.tq.ModifyLease(task, queueName, leaseTime))
}

func (t *tqCounter) Purge(queueName string) error {
	return t.c.Purge.up(t.tq.Purge(queueName))
}

func (t *tqCounter) Stats(queueNames []string, cb tq.RawStatsCB) error {
	return t.c.Stats.up(t.tq.Stats(queueNames, cb))
}

func (t *tqCounter) Constraints() tq.Constraints {
	return t.tq.Constraints()
}

func (t *tqCounter) GetTestable() tq.Testable {
	return t.tq.GetTestable()
}

// FilterTQ installs a counter TaskQueue filter in the context.
func FilterTQ(c context.Context) (context.Context, *TQCounter) {
	state := &TQCounter{}
	return tq.AddRawFilters(c, func(ic context.Context, tq tq.RawInterface) tq.RawInterface {
		return &tqCounter{state, tq}
	}), state
}
