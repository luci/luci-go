// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package featureBreaker

import (
	"time"

	"golang.org/x/net/context"

	tq "go.chromium.org/gae/service/taskqueue"
)

type tqState struct {
	*state

	c  context.Context
	tq tq.RawInterface
}

var _ tq.RawInterface = (*tqState)(nil)

func (t *tqState) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	if len(tasks) == 0 {
		return nil
	}
	return t.run(t.c, func() (err error) { return t.tq.AddMulti(tasks, queueName, cb) })
}

func (t *tqState) DeleteMulti(tasks []*tq.Task, queueName string, cb tq.RawCB) error {
	if len(tasks) == 0 {
		return nil
	}
	return t.run(t.c, func() error { return t.tq.DeleteMulti(tasks, queueName, cb) })
}

func (t *tqState) Lease(maxTasks int, queueName string, leaseTime time.Duration) (tasks []*tq.Task, err error) {
	err = t.run(t.c, func() (err error) {
		tasks, err = t.tq.Lease(maxTasks, queueName, leaseTime)
		return
	})
	if err != nil {
		tasks = nil
	}
	return
}

func (t *tqState) LeaseByTag(maxTasks int, queueName string, leaseTime time.Duration, tag string) (tasks []*tq.Task, err error) {
	err = t.run(t.c, func() (err error) {
		tasks, err = t.tq.LeaseByTag(maxTasks, queueName, leaseTime, tag)
		return
	})
	if err != nil {
		tasks = nil
	}
	return
}

func (t *tqState) ModifyLease(task *tq.Task, queueName string, leaseTime time.Duration) error {
	return t.run(t.c, func() error { return t.tq.ModifyLease(task, queueName, leaseTime) })
}

func (t *tqState) Purge(queueName string) error {
	return t.run(t.c, func() error { return t.tq.Purge(queueName) })
}

func (t *tqState) Stats(queueNames []string, cb tq.RawStatsCB) error {
	return t.run(t.c, func() error { return t.tq.Stats(queueNames, cb) })
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
		return &tqState{state, ic, tq}
	}), state
}
