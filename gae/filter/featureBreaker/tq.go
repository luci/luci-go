// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"golang.org/x/net/context"

	tq "github.com/luci/gae/service/taskqueue"
)

type tqState struct {
	*state

	tq tq.Interface
}

var _ tq.Interface = (*tqState)(nil)

func (t *tqState) Add(task *tq.Task, queueName string) (ret *tq.Task, err error) {
	err = t.run(func() (err error) {
		ret, err = t.tq.Add(task, queueName)
		return
	})
	return
}

func (t *tqState) Delete(task *tq.Task, queueName string) error {
	return t.run(func() error {
		return t.tq.Delete(task, queueName)
	})
}

func (t *tqState) AddMulti(tasks []*tq.Task, queueName string) (ret []*tq.Task, err error) {
	err = t.run(func() (err error) {
		ret, err = t.tq.AddMulti(tasks, queueName)
		return
	})
	return
}

func (t *tqState) DeleteMulti(tasks []*tq.Task, queueName string) error {
	return t.run(func() error {
		return t.tq.DeleteMulti(tasks, queueName)
	})
}

func (t *tqState) Lease(maxTasks int, queueName string, leaseTime int) (ret []*tq.Task, err error) {
	err = t.run(func() (err error) {
		ret, err = t.tq.Lease(maxTasks, queueName, leaseTime)
		return
	})
	return
}

func (t *tqState) LeaseByTag(maxTasks int, queueName string, leaseTime int, tag string) (ret []*tq.Task, err error) {
	err = t.run(func() (err error) {
		ret, err = t.tq.LeaseByTag(maxTasks, queueName, leaseTime, tag)
		return
	})
	return
}

func (t *tqState) ModifyLease(task *tq.Task, queueName string, leaseTime int) error {
	return t.run(func() error {
		return t.tq.ModifyLease(task, queueName, leaseTime)
	})
}

func (t *tqState) Purge(queueName string) error {
	return t.run(func() error {
		return t.tq.Purge(queueName)
	})
}

func (t *tqState) QueueStats(queueNames []string) (ret []tq.Statistics, err error) {
	err = t.run(func() (err error) {
		ret, err = t.tq.QueueStats(queueNames)
		return
	})
	return
}

// FilterTQ installs a counter TaskQueue filter in the context.
func FilterTQ(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return tq.AddFilters(c, func(ic context.Context, tq tq.Interface) tq.Interface {
		return &tqState{state, tq}
	}), state
}
