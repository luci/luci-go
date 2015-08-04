// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"fmt"
	"reflect"

	tq "github.com/luci/gae/service/taskqueue"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/taskqueue"
)

// useTQ adds a gae.TaskQueue implementation to context, accessible
// by gae.GetTQ(c)
func useTQ(c context.Context) context.Context {
	return tq.SetRawFactory(c, func(ci context.Context) tq.RawInterface {
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

// tqR2F (TQ real-to-fake) converts a *taskqueue.Task to a *tq.Task.
func tqR2F(o *taskqueue.Task) *tq.Task {
	if o == nil {
		return nil
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
	return &n
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

// tqMF2R (TQ multi-fake-to-real) converts []*tq.Task to []*taskqueue.Task.
func tqMF2R(ns []*tq.Task) []*taskqueue.Task {
	ret := make([]*taskqueue.Task, len(ns))
	for i, t := range ns {
		ret[i] = tqF2R(t)
	}
	return ret
}

func (t tqImpl) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	realTasks, err := taskqueue.AddMulti(t.Context, tqMF2R(tasks), queueName)
	if err != nil {
		if me, ok := err.(appengine.MultiError); ok {
			for i, err := range me {
				tsk := (*taskqueue.Task)(nil)
				if realTasks != nil {
					tsk = realTasks[i]
				}
				cb(tqR2F(tsk), err)
			}
			err = nil
		}
	} else {
		for _, tsk := range realTasks {
			cb(tqR2F(tsk), nil)
		}
	}
	return err
}

func (t tqImpl) DeleteMulti(tasks []*tq.Task, queueName string, cb tq.RawCB) error {
	err := taskqueue.DeleteMulti(t.Context, tqMF2R(tasks), queueName)
	if me, ok := err.(appengine.MultiError); ok {
		for _, err := range me {
			cb(err)
		}
		err = nil
	}
	return err
}

func (t tqImpl) Purge(queueName string) error {
	return taskqueue.Purge(t.Context, queueName)
}

func (t tqImpl) Stats(queueNames []string, cb tq.RawStatsCB) error {
	stats, err := taskqueue.QueueStats(t.Context, queueNames)
	if err != nil {
		return err
	}
	for _, s := range stats {
		cb((*tq.Statistics)(&s), nil)
	}
	return nil
}
