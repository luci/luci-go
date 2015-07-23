// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	tq "github.com/luci/gae/service/taskqueue"
)

// TQCounter is the counter object for the TaskQueue service.
type TQCounter struct {
	Add         Entry
	Delete      Entry
	AddMulti    Entry
	DeleteMulti Entry
	Lease       Entry
	LeaseByTag  Entry
	ModifyLease Entry
	Purge       Entry
	QueueStats  Entry
}

type tqCounter struct {
	c *TQCounter

	tq tq.Interface
}

var _ tq.Interface = (*tqCounter)(nil)

func (t *tqCounter) Add(task *tq.Task, queueName string) (*tq.Task, error) {
	ret, err := t.tq.Add(task, queueName)
	return ret, t.c.Add.up(err)
}

func (t *tqCounter) Delete(task *tq.Task, queueName string) error {
	return t.c.Delete.up(t.tq.Delete(task, queueName))
}

func (t *tqCounter) AddMulti(tasks []*tq.Task, queueName string) ([]*tq.Task, error) {
	ret, err := t.tq.AddMulti(tasks, queueName)
	return ret, t.c.AddMulti.up(err)
}

func (t *tqCounter) DeleteMulti(tasks []*tq.Task, queueName string) error {
	return t.c.DeleteMulti.up(t.tq.DeleteMulti(tasks, queueName))
}

func (t *tqCounter) Lease(maxTasks int, queueName string, leaseTime int) ([]*tq.Task, error) {
	ret, err := t.tq.Lease(maxTasks, queueName, leaseTime)
	return ret, t.c.Lease.up(err)
}

func (t *tqCounter) LeaseByTag(maxTasks int, queueName string, leaseTime int, tag string) ([]*tq.Task, error) {
	ret, err := t.tq.LeaseByTag(maxTasks, queueName, leaseTime, tag)
	return ret, t.c.LeaseByTag.up(err)
}

func (t *tqCounter) ModifyLease(task *tq.Task, queueName string, leaseTime int) error {
	return t.c.ModifyLease.up(t.tq.ModifyLease(task, queueName, leaseTime))
}

func (t *tqCounter) Purge(queueName string) error {
	return t.c.Purge.up(t.tq.Purge(queueName))
}

func (t *tqCounter) QueueStats(queueNames []string) ([]tq.Statistics, error) {
	ret, err := t.tq.QueueStats(queueNames)
	return ret, t.c.QueueStats.up(err)
}

// FilterTQ installs a counter TaskQueue filter in the context.
func FilterTQ(c context.Context) (context.Context, *TQCounter) {
	state := &TQCounter{}
	return tq.AddFilters(c, func(ic context.Context, tq tq.Interface) tq.Interface {
		return &tqCounter{state, tq}
	}), state
}
