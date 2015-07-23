// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"fmt"
	"reflect"

	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
	"google.golang.org/appengine/taskqueue"
)

// useTQ adds a gae.TaskQueue implementation to context, accessible
// by gae.GetTQ(c)
func useTQ(c context.Context) context.Context {
	return tq.SetFactory(c, func(ci context.Context) tq.Interface {
		return tqImpl{ci}
	})
}

type tqImpl struct {
	context.Context
}

func init() {
	const taskExpectedFields = 9
	// Runtime-assert that the number of fields in the Task structs is 9, to
	// avoid missing additional fields if they're added later.
	// all other type assertions are statically enforced by o2n() and tqF2R()

	oldType := reflect.TypeOf((*taskqueue.Task)(nil))
	newType := reflect.TypeOf((*tq.Task)(nil))

	if oldType.NumField() != newType.NumField() ||
		oldType.NumField() != taskExpectedFields {
		panic(fmt.Errorf(
			"prod/taskqueue:init() field count differs: %v, %v",
			oldType, newType))
	}
}

// tqR2FErr (TQ real-to-fake w/ error) converts a *taskqueue.Task to a
// *tq.Task, and passes through an error.
func tqR2FErr(o *taskqueue.Task, err error) (*tq.Task, error) {
	if err != nil {
		return nil, err
	}
	n := tq.Task{}
	n.Path = o.Path
	n.Payload = o.Payload
	n.Header = o.Header
	n.Method = o.Method
	n.Name = o.Name
	n.Delay = o.Delay
	n.ETA = o.ETA
	n.RetryCount = o.RetryCount
	n.RetryOptions = (*tq.RetryOptions)(o.RetryOptions)
	return &n, nil
}

// tqF2R (TQ fake-to-real) converts a *tq.Task to a *taskqueue.Task.
func tqF2R(n *tq.Task) *taskqueue.Task {
	o := taskqueue.Task{}
	o.Path = n.Path
	o.Payload = n.Payload
	o.Header = n.Header
	o.Method = n.Method
	o.Name = n.Name
	o.Delay = n.Delay
	o.ETA = n.ETA
	o.RetryCount = n.RetryCount
	o.RetryOptions = (*taskqueue.RetryOptions)(n.RetryOptions)
	return &o
}

// tqMR2FErr (TQ multi-real-to-fake w/ error) converts a slice of
// *taskqueue.Task to a slice of *tq.Task
func tqMR2FErr(os []*taskqueue.Task, err error) ([]*tq.Task, error) {
	if err != nil {
		return nil, errors.Fix(err)
	}
	ret := make([]*tq.Task, len(os))
	for i, t := range os {
		ret[i], _ = tqR2FErr(t, nil)
	}
	return ret, nil
}

// tqMF2R (TQ multi-fake-to-real) converts []*tq.Task to []*taskqueue.Task.
func tqMF2R(ns []*tq.Task) []*taskqueue.Task {
	ret := make([]*taskqueue.Task, len(ns))
	for i, t := range ns {
		ret[i] = tqF2R(t)
	}
	return ret
}

//////// TQSingleReadWriter

func (t tqImpl) Add(task *tq.Task, queueName string) (*tq.Task, error) {
	return tqR2FErr(taskqueue.Add(t.Context, tqF2R(task), queueName))
}
func (t tqImpl) Delete(task *tq.Task, queueName string) error {
	return taskqueue.Delete(t.Context, tqF2R(task), queueName)
}

//////// TQMultiReadWriter

func (t tqImpl) AddMulti(tasks []*tq.Task, queueName string) ([]*tq.Task, error) {
	return tqMR2FErr(taskqueue.AddMulti(t.Context, tqMF2R(tasks), queueName))
}
func (t tqImpl) DeleteMulti(tasks []*tq.Task, queueName string) error {
	return errors.Fix(taskqueue.DeleteMulti(t.Context, tqMF2R(tasks), queueName))
}

//////// TQLeaser

func (t tqImpl) Lease(maxTasks int, queueName string, leaseTime int) ([]*tq.Task, error) {
	return tqMR2FErr(taskqueue.Lease(t.Context, maxTasks, queueName, leaseTime))
}
func (t tqImpl) LeaseByTag(maxTasks int, queueName string, leaseTime int, tag string) ([]*tq.Task, error) {
	return tqMR2FErr(taskqueue.LeaseByTag(t.Context, maxTasks, queueName, leaseTime, tag))
}
func (t tqImpl) ModifyLease(task *tq.Task, queueName string, leaseTime int) error {
	return taskqueue.ModifyLease(t.Context, tqF2R(task), queueName, leaseTime)
}

//////// TQPurger

func (t tqImpl) Purge(queueName string) error {
	return taskqueue.Purge(t.Context, queueName)
}

//////// TQStatter

func (t tqImpl) QueueStats(queueNames []string) ([]tq.Statistics, error) {
	stats, err := taskqueue.QueueStats(t.Context, queueNames)
	if err != nil {
		return nil, err
	}
	ret := make([]tq.Statistics, len(stats))
	for i, s := range stats {
		ret[i] = tq.Statistics(s)
	}
	return ret, nil
}
