// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	"infra/gae/libs/gae"
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

	tq gae.TaskQueue
}

var _ gae.TaskQueue = (*tqCounter)(nil)

func (t *tqCounter) Add(task *gae.TQTask, queueName string) (*gae.TQTask, error) {
	ret, err := t.tq.Add(task, queueName)
	return ret, t.c.Add.up(err)
}

func (t *tqCounter) Delete(task *gae.TQTask, queueName string) error {
	return t.c.Delete.up(t.tq.Delete(task, queueName))
}

func (t *tqCounter) AddMulti(tasks []*gae.TQTask, queueName string) ([]*gae.TQTask, error) {
	ret, err := t.tq.AddMulti(tasks, queueName)
	return ret, t.c.AddMulti.up(err)
}

func (t *tqCounter) DeleteMulti(tasks []*gae.TQTask, queueName string) error {
	return t.c.DeleteMulti.up(t.tq.DeleteMulti(tasks, queueName))
}

func (t *tqCounter) Lease(maxTasks int, queueName string, leaseTime int) ([]*gae.TQTask, error) {
	ret, err := t.tq.Lease(maxTasks, queueName, leaseTime)
	return ret, t.c.Lease.up(err)
}

func (t *tqCounter) LeaseByTag(maxTasks int, queueName string, leaseTime int, tag string) ([]*gae.TQTask, error) {
	ret, err := t.tq.LeaseByTag(maxTasks, queueName, leaseTime, tag)
	return ret, t.c.LeaseByTag.up(err)
}

func (t *tqCounter) ModifyLease(task *gae.TQTask, queueName string, leaseTime int) error {
	return t.c.ModifyLease.up(t.tq.ModifyLease(task, queueName, leaseTime))
}

func (t *tqCounter) Purge(queueName string) error {
	return t.c.Purge.up(t.tq.Purge(queueName))
}

func (t *tqCounter) QueueStats(queueNames []string) ([]gae.TQStatistics, error) {
	ret, err := t.tq.QueueStats(queueNames)
	return ret, t.c.QueueStats.up(err)
}

// FilterTQ installs a counter TaskQueue filter in the context.
func FilterTQ(c context.Context) (context.Context, *TQCounter) {
	state := &TQCounter{}
	return gae.AddTQFilters(c, func(ic context.Context, tq gae.TaskQueue) gae.TaskQueue {
		return &tqCounter{state, tq}
	}), state
}
